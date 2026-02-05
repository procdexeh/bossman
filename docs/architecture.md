# Bossman Architecture

## Overview

Bossman is a task management system with three interfaces sharing a single SQLite database:

1. **CLI** - Quick task capture (`bossman add "thing"`)
2. **MCP Server** - AI client integration (Claude, Cursor)
3. **HTTP Server** - Web dashboard (read-only)

---

## Mental Model: Three Modes

```
+-----------------+     +-----------------+     +-----------------+
|   CLI Mode      |     |   MCP Mode      |     |   HTTP Mode     |
|                 |     |                 |     |                 |
|  You run it     |     |  AI runs it     |     |  You run it     |
|  One-shot       |     |  Per-session    |     |  Long-running   |
|  Exits after    |     |  Killed when    |     |  Background     |
|                 |     |  session ends   |     |  daemon         |
+-----------------+     +-----------------+     +-----------------+
         |                      |                       |
         +----------------------+-----------------------+
                               |
                               v
                  +------------------------+
                  |  ~/.bossman/bossman.db |
                  |  Persists across all   |
                  +------------------------+
```

### CLI Mode

```sh
bossman add "Fix parser bug"    # Create task, exit
bossman list                    # Show tasks, exit
bossman done task_abc123        # Mark complete, exit
```

- **Who starts it**: You
- **Lifetime**: One-shot command, exits immediately
- **Use case**: Quick capture without AI mediation

### MCP Mode

```sh
# You don't run this directly. AI clients do.
bossman mcp
```

- **Who starts it**: AI client (Claude, Cursor) via config
- **Lifetime**: Lives for duration of AI session
- **Transport**: stdio (JSON-RPC 2.0)
- **Use case**: AI-powered task management

MCP servers follow the **LSP pattern**: your editor doesn't require you to manually run a language server - it spawns one automatically when needed. Same with MCP.

### HTTP Mode

```sh
bossman serve --port 8080
```

- **Who starts it**: You (optional)
- **Lifetime**: Long-running daemon
- **Use case**: Web dashboard, read-only task visualization

---

## Single Binary Architecture

One binary, multiple modes via subcommands:

```
bossman              # Show help / quick status
bossman add <desc>   # CLI: create task
bossman list         # CLI: list tasks
bossman done <id>    # CLI: mark complete
bossman mcp          # MCP server (stdio)
bossman serve        # HTTP server (web UI)
```

### Project Structure

```
bossman/
+-- cmd/
|   +-- bossman/
|       +-- main.go             # Entry point, subcommand dispatch
+-- internal/
|   +-- cli/
|   |   +-- add.go              # bossman add
|   |   +-- list.go             # bossman list
|   |   +-- done.go             # bossman done
|   +-- db/
|   |   +-- db.go               # SQLite layer (shared)
|   +-- mcp/
|   |   +-- server.go           # MCP lifecycle, dispatch
|   |   +-- transport.go        # stdio JSON-RPC
|   |   +-- types.go            # Protocol types
|   |   +-- errors.go           # JSON-RPC error codes
|   +-- http/
|   |   +-- server.go           # HTTP server
|   |   +-- handlers.go         # Routes
|   +-- tools/
|       +-- registry.go         # Tool registration
|       +-- tasks.go            # Task CRUD tools
|       +-- blockers.go         # Dependency tools
+-- docs/
|   +-- architecture.md         # This file
+-- go.mod
```

### Entry Point

```go
// cmd/bossman/main.go
func main() {
    if len(os.Args) < 2 {
        printUsage()
        return
    }

    switch os.Args[1] {
    case "mcp":
        mcp.Run()           // stdio JSON-RPC server
    case "serve":
        http.Run()          // HTTP daemon
    case "add":
        cli.Add(os.Args[2:])
    case "list":
        cli.List(os.Args[2:])
    case "done":
        cli.Done(os.Args[2:])
    default:
        printUsage()
    }
}
```

---

## Data Persistence

All data lives in `~/.bossman/`:

```
~/.bossman/
    config.toml      # Optional settings
    bossman.db       # SQLite database (WAL mode)
```

### Why `~/.bossman/` Over XDG

- Simpler mental model
- Easy to backup: `cp -r ~/.bossman ~/backup/`
- Common pattern: `~/.docker/`, `~/.cargo/`, `~/.npm/`

### Environment Overrides

```sh
BOSSMAN_DB_PATH=/custom/path/bossman.db
BOSSMAN_CONFIG=/custom/path/config.toml
```

### Config File (Optional)

```toml
# ~/.bossman/config.toml

[server]
port = 8080
host = "localhost"

[defaults]
priority = 3
```

---

## Concurrency Model

SQLite with WAL mode allows concurrent readers from different processes:

```
+--------------+  +--------------+  +---------------+
| CLI process  |  | MCP process  |  | HTTP process  |
+------+-------+  +------+-------+  +-------+-------+
       |                 |                  |
       +-----------------+------------------+
                         |
                         v
               +-------------------+
               |  bossman.db (WAL) |
               |  SetMaxOpenConns(1)
               +-------------------+
```

- Each process opens its own connection
- WAL mode: readers don't block writers
- `SetMaxOpenConns(1)`: serializes writes within each process
- Good enough for task management workloads

---

## Interface Responsibilities

| Interface | Read | Write | Use Case |
|-----------|------|-------|----------|
| CLI | Yes | Yes | Quick capture, scripting |
| MCP | Yes | Yes | AI-powered management |
| HTTP | Yes | No | Dashboard visualization |

HTTP is intentionally read-only to avoid:
- Auth complexity
- Conflict with AI-driven updates
- Scope creep

---

## MCP Protocol Implementation

### Protocol Reference

- **Spec**: `2025-03-26` - [modelcontextprotocol.io/specification/2025-03-26](https://modelcontextprotocol.io/specification/2025-03-26)
- **Transport**: stdio (newline-delimited JSON-RPC 2.0)
- **Server features**: Tools, Logging
- **Consumer**: Cloudflare Code Mode (tool schemas -> TypeScript APIs -> V8 sandbox)

### MCP Client Configuration

AI clients configure MCP servers in their settings. Example for Claude:

```json
{
  "mcpServers": {
    "bossman": {
      "command": "bossman",
      "args": ["mcp"],
      "env": {
        "BOSSMAN_DB_PATH": "~/.bossman/bossman.db"
      }
    }
  }
}
```

When you start a Claude session:
1. Claude reads this config
2. Spawns `bossman mcp` as a subprocess
3. Communicates via stdin/stdout (JSON-RPC)
4. Kills the process when session ends

**You never interact with `bossman mcp` directly.**

### Server State Machine

```
CREATED --initialize--> INITIALIZING --notifications/initialized--> OPERATING --EOF/SIGTERM--> SHUTDOWN
```

### Protocol Messages

#### Requests Server Handles

| Method            | Allowed States    | Notes                                |
|-------------------|-------------------|--------------------------------------|
| `initialize`      | CREATED only      | Rejects duplicate calls              |
| `ping`            | ALL states        | Returns `{}`                         |
| `tools/list`      | OPERATING         | Returns all tool definitions         |
| `tools/call`      | OPERATING         | Executes tool, returns result        |
| `logging/setLevel`| OPERATING         | Sets minimum log verbosity           |

#### Notifications Server Handles

| Method                         | Effect                               |
|--------------------------------|--------------------------------------|
| `notifications/initialized`    | Transitions INITIALIZING -> OPERATING|
| `notifications/cancelled`      | Cancels in-flight request context    |

#### Notifications Server Sends

| Method                         | When                                 |
|--------------------------------|--------------------------------------|
| `notifications/message`        | Structured log events to client      |

### Tool Inventory

| Tool              | Description                  | Required Args                  | Optional Args                                |
|-------------------|------------------------------|--------------------------------|----------------------------------------------|
| `create_task`     | Create a new task            | `description`                  | `parent_id`, `priority`, `context`           |
| `list_tasks`      | List tasks with filters      | --                             | `status`, `parent_id`, `limit`               |
| `get_task`        | Get task by ID               | `id`                           | --                                           |
| `update_task`     | Update task fields           | `id`                           | `description`, `priority`, `status`, `context`, `result` |
| `delete_task`     | Delete a task                | `id`                           | --                                           |
| `add_blocker`     | Add task dependency          | `task_id`, `blocked_by_id`     | --                                           |
| `remove_blocker`  | Remove dependency            | `task_id`, `blocked_by_id`     | --                                           |
| `get_blockers`    | List blockers for a task     | `task_id`                      | --                                           |

### JSON Schema Pattern for Code Mode

Code Mode converts `inputSchema` into TypeScript interfaces. Schemas MUST:

- Use `"additionalProperties": false` (prevents TS from allowing arbitrary fields)
- Include `"description"` on every property (becomes JSDoc in generated TS)
- Use `"enum"` arrays for constrained values (generates TS union types)
- Declare `"required"` fields explicitly

---

## Database Layer

### Opening with WAL Mode

```go
func Open(path string) (*sqlx.DB, error) {
    db, err := sqlx.Connect("sqlite",
        path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON",
    )
    if err != nil {
        return nil, fmt.Errorf("open database: %w", err)
    }
    db.SetMaxOpenConns(1)
    return db, nil
}
```

### Query Functions

All query functions accept `context.Context` so MCP cancellation propagates to the DB:

```go
func InsertTask(ctx context.Context, db *sqlx.DB, t *Task) error
func QueryTasks(ctx context.Context, db *sqlx.DB, opts ListOpts) ([]Task, error)
func GetTask(ctx context.Context, db *sqlx.DB, id string) (*Task, error)
func UpdateTask(ctx context.Context, db *sqlx.DB, id string, ...) error
func DeleteTask(ctx context.Context, db *sqlx.DB, id string) error
func TaskExists(ctx context.Context, db *sqlx.DB, id string) (bool, error)

func AddBlocker(ctx context.Context, db *sqlx.DB, taskID, blockedByID string) error
func RemoveBlocker(ctx context.Context, db *sqlx.DB, taskID, blockedByID string) error
func GetBlockers(ctx context.Context, db *sqlx.DB, taskID string) ([]Task, error)
```

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Binary architecture | Single binary, subcommands | One artifact, version consistency |
| MCP transport | stdio only | Matches LSP pattern, avoids auth |
| Data location | `~/.bossman/` | Simple, portable, easy backup |
| HTTP scope | Read-only | Dashboard only, no edit complexity |
| DB concurrency | `SetMaxOpenConns(1)` | Simple, sufficient for workload |
| CLI scope | Minimal (add/list/done) | Quick capture, not full TUI |

---

## Implementation Status

| Component | Status | Location |
|-----------|--------|----------|
| SQLite schema | Done | `main.go` (to extract) |
| Task model | Done | `main.go` (to extract) |
| HTTP server | Partial | `main.go` |
| CLI commands | Not started | `internal/cli/` |
| MCP server | Not started | `internal/mcp/` |
| MCP tools | Not started | `internal/tools/` |
| DB layer extraction | Not started | `internal/db/` |
| Config file support | Not started | - |
| `~/.bossman/` directory | Not started | - |

### Migration Path

1. Extract DB layer to `internal/db/`
2. Add `~/.bossman/` directory support with env override
3. Add subcommand dispatch in `cmd/bossman/main.go`
4. Implement minimal CLI (`add`, `list`, `done`)
5. Implement MCP server + tools
6. Migrate HTTP server to `internal/http/`

### MCP Implementation Order

| Phase | Work                                                  | Depends On       |
|-------|-------------------------------------------------------|------------------|
| 1     | `internal/mcp/types.go` - protocol types + RequestID  | --               |
| 2     | `internal/mcp/errors.go` - error codes + constructors | types            |
| 3     | `internal/mcp/transport.go` - stdio + batch support   | types            |
| 4     | `internal/mcp/server.go` - lifecycle + dispatch       | transport, errors|
| 5     | `internal/db/db.go` - extract from main.go, WAL mode  | --               |
| 6     | `internal/tools/registry.go` - handler interface      | mcp types        |
| 7     | `internal/tools/tasks.go` + `blockers.go` - impls     | registry, db     |
| 8     | `cmd/mcp/main.go` - wire everything                   | all              |
| 9     | Tests                                                 | all              |

---

## Testing Strategy

### Unit Tests

- **Transport**: Feed known JSON lines (single + batch) to `Transport` with `bytes.Buffer`, verify parse and batch discrimination
- **RequestID**: Round-trip string IDs, number IDs, reject null
- **Dispatch**: Mock `ToolHandler`, verify routing, state guards, duplicate initialize rejection
- **Lifecycle**: State transitions, reject requests in wrong state

### Integration Tests

- Spawn `cmd/mcp` as subprocess
- Write JSON-RPC to stdin, read stdout
- Full flow: `initialize` -> `initialized` -> `tools/list` -> `tools/call` -> EOF
- Cancellation: send request + immediate cancel, verify context cancellation
- Batch: send `[{tools/list}, {ping}]`, verify batch response

### Conformance

Validate against the official MCP Inspector:

```sh
npx @modelcontextprotocol/inspector go run ./cmd/mcp
```

---

## Future Considerations

**Not solving now:**
- Multi-user / hosted deployment (needs Postgres, auth)
- HTTP/SSE transport for remote MCP (use SSH tunneling instead)
- Full TUI interface (CLI is for quick capture only)
- Mobile app (HTTP API could support later)

**When to reconsider:**
- Performance issues with SQLite -> measure first
- Need remote MCP access -> SSH tunneling before HTTP transport
- Multiple users -> different architecture entirely

---

## Unresolved Questions

1. **go.mod dependencies** - Need `golang.org/x/time/rate` for rate limiter. Alternative: simple token bucket without the dependency?

2. **Tool schema source of truth** - Hand-written JSON Schema strings in Go. Acceptable for 8 tools, but schema drift is a risk. Alternative: define schemas as `.json` files, embed with `go:embed`. Tradeoff is indirection vs safety.

3. **Output sanitization** - Currently returning full `Task` struct as JSON. Filter fields (e.g. hide internal timestamps)? Or full transparency for Code Mode TS types?

4. **Error message verbosity** - Tool execution errors go to the LLM. Terse machine-readable strings or descriptive human-readable messages?

5. **`SetMaxOpenConns(1)`** - Serializes ALL operations including reads. In WAL mode, reads don't block writes. Separate read/write pools worth the complexity?
