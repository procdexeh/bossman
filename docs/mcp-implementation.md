# MCP Implementation Guide

Hand-rolled stdio MCP server. No SDK. Spec version `2025-03-26`.

Full spec: https://modelcontextprotocol.io/specification/2025-03-26

---

## Stdio Transport

- Read JSON-RPC from **stdin**, write to **stdout**
- Messages are **newline-delimited**, no embedded newlines
- Each message is either a single JSON object `{...}` or a batch `[{...}, {...}]`
- stderr is free for logging (slog goes there)
- Shutdown: client closes stdin -> EOF -> exit cleanly

---

## Lifecycle State Machine

```
CREATED ──initialize──> INITIALIZING ──notifications/initialized──> OPERATING ──EOF──> SHUTDOWN
```

- `initialize`: only valid in CREATED. Respond with protocolVersion + capabilities + serverInfo
- `notifications/initialized`: notification (no response). Transitions to OPERATING
- `ping`: valid in ALL states. Respond with `{}`
- `tools/list`, `tools/call`: only valid in OPERATING
- Duplicate `initialize`: reject with `-32600`

---

## Messages Server Handles

| Method                       | Type         | Allowed States | Response                                   |
|------------------------------|--------------|----------------|--------------------------------------------|
| `initialize`                 | request      | CREATED only   | `InitializeResult`                         |
| `notifications/initialized`  | notification | INITIALIZING   | none                                       |
| `ping`                       | request      | any            | `{}`                                       |
| `tools/list`                 | request      | OPERATING      | `{ "tools": [...] }`                       |
| `tools/call`                 | request      | OPERATING      | `{ "content": [...], "isError": bool }`    |
| `notifications/cancelled`    | notification | any            | none (cancel context for inflight request) |

---

## JSON-RPC 2.0 Essentials

### Message Types

**Request** (has `id`, expects response):

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {}
}
```

**Notification** (no `id`, no response):

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

**Response** (echoes `id` back, exact same type):

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {}
}
```

### Request ID

- Can be **string** or **number**. Never null.
- Response MUST echo back the ID with the **exact same type** (string stays string, number stays number).

### Error Codes

| Code   | Meaning         |
|--------|-----------------|
| -32700 | Parse error     |
| -32600 | Invalid request |
| -32601 | Method not found|
| -32602 | Invalid params  |
| -32603 | Internal error  |

### Batch

A batch is a JSON array of requests/notifications:

```json
[
  {"jsonrpc": "2.0", "id": 1, "method": "tools/list"},
  {"jsonrpc": "2.0", "id": 2, "method": "ping"}
]
```

Response is a JSON array of responses (same order not required). Notifications in a batch produce no response entry.

---

## Protocol Flow

### Initialize Handshake

Client sends:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-03-26",
    "capabilities": {},
    "clientInfo": {
      "name": "ExampleClient",
      "version": "1.0.0"
    }
  }
}
```

Server responds:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "tools": {},
      "logging": {}
    },
    "serverInfo": {
      "name": "bossman",
      "version": "0.1.0"
    }
  }
}
```

Client sends (notification, no response):

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

Server is now in OPERATING state.

### tools/list

Client sends:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

Server responds:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "create_task",
        "description": "Create a new task",
        "inputSchema": {
          "type": "object",
          "properties": {
            "description": {
              "type": "string",
              "description": "Task description"
            }
          },
          "required": ["description"],
          "additionalProperties": false
        }
      }
    ]
  }
}
```

### tools/call

Client sends:

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "create_task",
    "arguments": {
      "description": "Fix parser bug"
    }
  }
}
```

Server responds (success):

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"id\":\"task_abc123\",\"description\":\"Fix parser bug\",\"status\":\"pending\"}"
      }
    ],
    "isError": false
  }
}
```

Server responds (tool execution error -- NOT a protocol error):

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "error: task not found: task_xyz"
      }
    ],
    "isError": true
  }
}
```

### Protocol Error (unknown tool, bad params)

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "error": {
    "code": -32602,
    "message": "Unknown tool: nonexistent_tool"
  }
}
```

---

## Cancellation

Client sends notification to cancel an inflight request:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/cancelled",
  "params": {
    "requestId": "123",
    "reason": "User requested cancellation"
  }
}
```

Server behavior:

- Cancel the `context.Context` for the inflight request matching `requestId`
- If request already completed or unknown, ignore silently
- Do NOT send a response for cancelled requests
- Race conditions are expected -- handle gracefully

---

## Tool Schemas

All `inputSchema` values are JSON Schema objects. For Code Mode compatibility:

- Always include `"additionalProperties": false` on every object schema
- Include `"description"` on every property (becomes JSDoc in generated TS)
- Use `"enum"` for constrained values (becomes TS union types)
- Declare `"required"` explicitly

### Two Error Flavors

1. **Protocol error**: JSON-RPC error response (unknown tool, bad params, wrong state)
2. **Execution error**: normal `result` with `"isError": true` (business logic failures, DB errors)

Protocol errors mean the tool call never executed. Execution errors mean it ran but failed.

---

## Implementation Order

| Phase | File                       | What                                            | Depends On        |
|-------|----------------------------|--------------------------------------------------|-------------------|
| 1     | `internal/mcp/types.go`    | RequestID (string\|number), Request, Response, state enum | --                |
| 2     | `internal/mcp/errors.go`   | Error codes + constructors                       | types             |
| 3     | `internal/mcp/transport.go`| stdin scanner, batch detection, write responses   | types             |
| 4     | `internal/mcp/server.go`   | State machine, dispatch, inflight cancellation    | transport, errors |

### Phase 1: types.go

- `RequestID`: wraps `interface{}` (string or float64). Custom `MarshalJSON`/`UnmarshalJSON`. Reject null.
- `Request`: `jsonrpc`, `id` (optional, nil = notification), `method`, `params` (json.RawMessage)
- `Response`: `jsonrpc`, `id`, `result` (json.RawMessage), `error`
- `ServerState`: int enum (Created, Initializing, Operating, Shutdown)
- Protocol types: `InitializeParams`, `InitializeResult`, `EntityInfo`, `Capabilities`, `ToolDefinition`, `ToolResult`, `ContentBlock`

### Phase 2: errors.go

- `Error` struct with `Code`, `Message`, `Data`
- Constructors: `NewParseError`, `NewInvalidRequest`, `NewMethodNotFound`, `NewInvalidParams`, `NewInternalError`

### Phase 3: transport.go

- `NewTransport(r io.Reader, w io.Writer)` -- takes stdin/stdout
- `ReadMessage() ([]Request, error)` -- scan one line, detect batch vs single by peeking first non-whitespace byte for `[`
- `WriteResponse(Response) error` -- marshal + newline
- `WriteBatchResponse([]Response) error` -- marshal array + newline
- Buffer size: 1MB max message

### Phase 4: server.go

- `NewServer(handler ToolHandler)` -- wires transport to stdin/stdout
- `Run() error` -- loop: read message, dispatch, write response. EOF = shutdown.
- `ToolHandler` interface: `ListTools() []ToolDefinition` + `CallTool(ctx, name, args) (*ToolResult, error)`
- State machine: check state before dispatch, reject wrong-state requests
- Inflight tracking: `map[string]context.CancelFunc` for cancellation support
- Dispatch table: `initialize`, `notifications/initialized`, `ping`, `tools/list`, `tools/call`, `notifications/cancelled`

---

## Testing

### Unit Tests

- **RequestID**: round-trip string IDs, number IDs, reject null
- **Transport**: feed known JSON lines to `ReadMessage` with `bytes.Buffer`, verify parse and batch detection
- **State machine**: reject requests in wrong state, duplicate initialize, proper transitions

### Integration

Spawn server as subprocess, write JSON-RPC to stdin, read stdout:

```
initialize -> initialized -> tools/list -> tools/call -> EOF
```

### Conformance

```sh
npx @modelcontextprotocol/inspector go run ./cmd/bossman
```
