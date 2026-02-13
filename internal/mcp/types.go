package mcp

import "encoding/json"

// Request is a JSON-RPC 2.0 request or notification.
// ID is nil for notifications.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification returns true if this message has no ID (notification).
func (r *Request) IsNotification() bool { return r.ID == nil }

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// NewResponse creates a success response echoing the request ID.
func NewResponse(id json.RawMessage, result json.RawMessage) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

// NewErrorResponse creates an error response echoing the request ID.
func NewErrorResponse(id json.RawMessage, e *Error) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: e}
}

type ServerState int

const (
	StateCreated ServerState = iota
	StateInitializing
	StateOperating
	StateShutdown
)

type EntityInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools   *struct{} `json:"tools,omitempty"`
	Logging *struct{} `json:"logging,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      EntityInfo   `json:"serverInfo"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type CancelParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Reason    string          `json:"reason"`
}
