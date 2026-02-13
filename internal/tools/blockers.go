package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"procdexeh/bossman/internal/db"
	"procdexeh/bossman/internal/mcp"
)

func (r *Registry) addBlocker(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		TaskID      string `json:"task_id"`
		BlockedByID string `json:"blocked_by_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if err := db.AddBlocker(ctx, r.db, params.TaskID, params.BlockedByID); err != nil {
		return nil, fmt.Errorf("add blocker: %w", err)
	}

	return resultJSON(map[string]string{
		"task_id":       params.TaskID,
		"blocked_by_id": params.BlockedByID,
		"status":        "added",
	})
}

func (r *Registry) removeBlocker(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		TaskID      string `json:"task_id"`
		BlockedByID string `json:"blocked_by_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	err := db.RemoveBlocker(ctx, r.db, params.TaskID, params.BlockedByID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("blocker not found: %s -> %s", params.TaskID, params.BlockedByID)
	}
	if err != nil {
		return nil, fmt.Errorf("remove blocker: %w", err)
	}

	return resultJSON(map[string]string{
		"task_id":       params.TaskID,
		"blocked_by_id": params.BlockedByID,
		"status":        "removed",
	})
}

func (r *Registry) getBlockers(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	tasks, err := db.GetBlockers(ctx, r.db, params.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get blockers: %w", err)
	}

	return resultJSON(tasks)
}

func (r *Registry) registerBlockerTools() {
	r.register(mcp.ToolDefinition{
		Name:        "add_blocker",
		Description: "Add a dependency between tasks",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "task_id": {
                    "type": "string",
                    "description": "The task that is blocked"
                },
                "blocked_by_id": {
                    "type": "string",
                    "description": "The task that is blocking"
                }
            },
            "required": ["task_id", "blocked_by_id"],
            "additionalProperties": false
        }`),
	}, r.addBlocker)

	r.register(mcp.ToolDefinition{
		Name:        "remove_blocker",
		Description: "Remove a dependency between tasks",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "task_id": {
                    "type": "string",
                    "description": "The task that is blocked"
                },
                "blocked_by_id": {
                    "type": "string",
                    "description": "The task that was blocking"
                }
            },
            "required": ["task_id", "blocked_by_id"],
            "additionalProperties": false
        }`),
	}, r.removeBlocker)

	r.register(mcp.ToolDefinition{
		Name:        "get_blockers",
		Description: "List tasks blocking a given task",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "task_id": {
                    "type": "string",
                    "description": "The task to get blockers for"
                }
            },
            "required": ["task_id"],
            "additionalProperties": false
        }`),
	}, r.getBlockers)
}
