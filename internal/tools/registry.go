package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"procdexeh/bossman/internal/mcp"
)

// signature every tool implementation must match
type toolFunc func(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error)

// registry holds tool definitions and their implementations
// it implements mcp.ToolHandler
type Registry struct {
	db    *sqlx.DB
	tools map[string]registeredTool
}

func (r *Registry) register(def mcp.ToolDefinition, fn toolFunc) {
	r.tools[def.Name] = registeredTool{def: def, invoke: fn}
}

func (r *Registry) ListTools() []mcp.ToolDefinition {
	defs := make([]mcp.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.def)
	}
	return defs
}

func (r *Registry) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcp.ToolResult, error) {
	took, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return took.invoke(ctx, args)
}

func (r *Registry) HasTool(name string) bool {
	_, ok := r.tools[name]
	return ok
}

type registeredTool struct {
	def    mcp.ToolDefinition
	invoke toolFunc
}

func NewRegistry(db *sqlx.DB) *Registry {
	r := &Registry{
		db:    db,
		tools: make(map[string]registeredTool),
	}
	r.registerTaskTools()
	r.registerBlockerTools()
	return r
}
