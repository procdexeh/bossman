package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
)

// ToolHandler is the boundary between protocol and business logic.
// Implement this to register tools the MCP server exposes to clients.
type ToolHandler interface {
	ListTools() []ToolDefinition
	CallTool(ctx context.Context, name string, args json.RawMessage) (*ToolResult, error)
}

// Server implements the MCP lifecycle over stdio.
// It manages the state machine (Created -> Initializing -> Operating -> Shutdown)
// and dispatches JSON-RPC requests to the appropriate handler.
type Server struct {
	transport *Transport
	handler   ToolHandler
	state     ServerState
	inflight  map[string]context.CancelFunc // tracks in-progress requests for cancellation
	mu        sync.Mutex                    // guards state and inflight
}

func (s *Server) handleInitialize(req Request) *Response {
	s.mu.Lock()
	s.state = StateInitializing
	s.mu.Unlock()

	result := InitializeResult{
		ProtocolVersion: "2025-03-26",
		Capabilities: Capabilities{
			Tools:   &struct{}{}, // &struct{}{} marshals to {} — "capability present, no config"
			Logging: &struct{}{},
		},
		ServerInfo: EntityInfo{
			Name:    "bossman",
			Version: "0.1.0",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		r := NewErrorResponse(req.ID, NewInternalError(err.Error()))
		return &r
	}
	r := NewResponse(req.ID, data)
	return &r
}

func (s *Server) handleToolsList(req Request) *Response {
	tools := s.handler.ListTools()

	// Anonymous struct wraps the slice to produce {"tools": [...]}
	result := struct {
		Tools []ToolDefinition `json:"tools"`
	}{Tools: tools}

	data, err := json.Marshal(result)
	if err != nil {
		r := NewErrorResponse(req.ID, NewInternalError(err.Error()))
		return &r
	}

	r := NewResponse(req.ID, data)
	return &r
}

func (s *Server) handleToolsCall(req Request) *Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		r := NewErrorResponse(req.ID, NewInvalidParams(err.Error()))
		return &r
	}

	// Cancellable context: stored in inflight so notifications/cancelled can stop this request.
	ctx, cancel := context.WithCancel(context.Background())
	key := string(req.ID)

	s.mu.Lock()
	s.inflight[key] = cancel
	s.mu.Unlock()

	result, err := s.handler.CallTool(ctx, params.Name, params.Arguments)

	// Cleanup: remove from inflight before cancelling to avoid a redundant cancel
	// from a racing notifications/cancelled.
	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()
	cancel() // release context resources even on the normal path

	// Tool errors are execution errors, not protocol errors.
	// They go in result with isError:true — the tool ran but failed.
	if err != nil {
		result = &ToolResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		r := NewErrorResponse(req.ID, NewInternalError(err.Error()))
		return &r
	}

	r := NewResponse(req.ID, data)
	return &r
}

// handleNotification processes messages with no ID (fire-and-forget, no response sent).
func (s *Server) handleNotification(req Request) {
	switch req.Method {
	case "notifications/initialized":
		s.mu.Lock()
		if s.state == StateInitializing {
			s.state = StateOperating
		}
		s.mu.Unlock()

	case "notifications/cancelled":
		var params CancelParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return // malformed cancel — spec says ignore silently
		}
		key := string(params.RequestID)
		s.mu.Lock()
		if cancel, ok := s.inflight[key]; ok {
			cancel()
			delete(s.inflight, key)
		}
		s.mu.Unlock()
	}
}

// dispatch routes a request to its handler after checking the state machine.
// Returns nil for notifications (no response needed).
func (s *Server) dispatch(req Request) *Response {
	if req.IsNotification() {
		s.handleNotification(req)
		return nil
	}

	// Snapshot state under lock, then release — handlers may be slow.
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()

	switch req.Method {
	case "initialize":
		if state != StateCreated {
			r := NewErrorResponse(req.ID, NewInvalidRequest("already initialized"))
			return &r
		}
		return s.handleInitialize(req)
	case "ping":
		r := NewResponse(req.ID, json.RawMessage(`{}`))
		return &r
	case "tools/list":
		if state != StateOperating {
			r := NewErrorResponse(req.ID, NewInvalidRequest("server not initialized"))
			return &r
		}
		return s.handleToolsList(req)
	case "tools/call":
		if state != StateOperating {
			r := NewErrorResponse(req.ID, NewInvalidRequest("server not initialized"))
			return &r
		}
		return s.handleToolsCall(req)
	default:
		r := NewErrorResponse(req.ID, NewMethodNotFound(req.Method))
		return &r
	}
}

func NewServer(handler ToolHandler) *Server {
	return &Server{
		transport: NewTransport(os.Stdin, os.Stdout),
		handler:   handler,
		state:     StateCreated,
		inflight:  make(map[string]context.CancelFunc),
	}
}

// Run is the main loop. Reads messages from stdin, dispatches, writes responses to stdout.
// Returns nil on clean shutdown (stdin EOF), error if the transport breaks.
func (s *Server) Run() error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	for {
		msgs, err := s.transport.ReadMessage()
		if err == io.EOF {
			s.mu.Lock()
			s.state = StateShutdown
			s.mu.Unlock()
			return nil
		}
		if err != nil {
			logger.Error("parse error", "err", err)
			// null ID: we couldn't parse the request, so we don't know the ID
			resp := NewErrorResponse(nil, NewParseError(err.Error()))
			if writeErr := s.transport.WriteResponse(resp); writeErr != nil {
				return writeErr
			}
			continue
		}

		if len(msgs) == 1 {
			resp := s.dispatch(msgs[0])
			if resp != nil {
				if err := s.transport.WriteResponse(*resp); err != nil {
					return err
				}
			}
		} else {
			// Batch: collect responses, skip nil (notifications), write as JSON array
			var responses []Response
			for _, msg := range msgs {
				resp := s.dispatch(msg)
				if resp != nil {
					responses = append(responses, *resp)
				}
			}
			if len(responses) > 0 {
				if err := s.transport.WriteBatchResponse(responses); err != nil {
					return err
				}
			}
		}
	}
}
