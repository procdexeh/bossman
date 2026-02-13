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

func resultJSON(v any) (*mcp.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.ToolResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func (r *Registry) listTasks(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		Status   *string `json:"status"`
		ParentID *string `json:"parent_id"`
		Limit    int     `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	tasks, err := db.QueryTasks(ctx, r.db, db.ListOpts{
		Status:   params.Status,
		ParentID: params.ParentID,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	return resultJSON(tasks)
}

func (r *Registry) getTask(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	task, err := db.GetTask(ctx, r.db, params.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task not found: %s", params.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return resultJSON(task)
}

func (r *Registry) deleteTask(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	err := db.DeleteTask(ctx, r.db, params.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task not found: %s", params.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("delete task: %w", err)
	}
	return resultJSON(map[string]string{"deleted": params.ID})
}

func (r *Registry) createTask(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		Description string  `json:"description"`
		ParentID    *string `json:"parent_id"`
		Priority    *int    `json:"priority"`
		Context     *string `json:"context"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	task := &db.Task{
		ID:          db.NewTaskID(),
		Description: params.Description,
		ParentID:    params.ParentID,
		Priority:    3, // default; CHECK constraint rejects 0
	}
	if params.Priority != nil {
		task.Priority = *params.Priority
	}
	if params.Context != nil {
		task.Context = *params.Context
	}
	if err := db.InsertTask(ctx, r.db, task); err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return resultJSON(task)
}

func (r *Registry) updateTask(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	var params struct {
		ID          string  `json:"id"`
		Description *string `json:"description"`
		Priority    *int    `json:"priority"`
		Status      *string `json:"status"`
		Context     *string `json:"context"`
		Result      *string `json:"result"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	err := db.UpdateTask(ctx, r.db, params.ID, db.UpdateOpts{
		Description: params.Description,
		Priority:    params.Priority,
		Status:      params.Status,
		Context:     params.Context,
		Result:      params.Result,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task not found: %s", params.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}

	// Return the updated task so the client sees the current state
	task, err := db.GetTask(ctx, r.db, params.ID)
	if err != nil {
		return nil, fmt.Errorf("get updated task: %w", err)
	}

	return resultJSON(task)
}

func (r *Registry) registerTaskTools() {
	r.register(mcp.ToolDefinition{
		Name:        "create_task",
		Description: "Create a new task",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "description": {
                    "type": "string",
                    "description": "Task description"
                },
                "parent_id": {
                    "type": "string",
                    "description": "Parent task ID for subtasks"
                },
                "priority": {
                    "type": "integer",
                    "description": "Priority 1-5 (1 is highest)",
                    "minimum": 1,
                    "maximum": 5
                },
                "context": {
                    "type": "string",
                    "description": "Additional context or notes"
                }
            },
            "required": ["description"],
            "additionalProperties": false
        }`),
	}, r.createTask)

	r.register(mcp.ToolDefinition{
		Name:        "list_tasks",
		Description: "List tasks with optional filters",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "status": {
                    "type": "string",
                    "description": "Filter by status",
                    "enum": ["pending", "in_progress", "completed", "failed"]
                },
                "parent_id": {
                    "type": "string",
                    "description": "Filter by parent task ID"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of tasks to return"
                }
            },
            "additionalProperties": false
        }`),
	}, r.listTasks)

	r.register(mcp.ToolDefinition{
		Name:        "get_task",
		Description: "Get a task by ID",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "id": {
                    "type": "string",
                    "description": "Task ID"
                }
            },
            "required": ["id"],
            "additionalProperties": false
        }`),
	}, r.getTask)

	r.register(mcp.ToolDefinition{
		Name:        "update_task",
		Description: "Update fields on an existing task",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "id": {
                    "type": "string",
                    "description": "Task ID"
                },
                "description": {
                    "type": "string",
                    "description": "Updated task description"
                },
                "priority": {
                    "type": "integer",
                    "description": "Priority 1-5 (1 is highest)",
                    "minimum": 1,
                    "maximum": 5
                },
                "status": {
                    "type": "string",
                    "description": "Task status",
                    "enum": ["pending", "in_progress", "completed", "failed"]
                },
                "context": {
                    "type": "string",
                    "description": "Additional context or notes"
                },
                "result": {
                    "type": "string",
                    "description": "Task result or outcome"
                }
            },
            "required": ["id"],
            "additionalProperties": false
        }`),
	}, r.updateTask)

	r.register(mcp.ToolDefinition{
		Name:        "delete_task",
		Description: "Delete a task by ID",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "id": {
                    "type": "string",
                    "description": "Task ID"
                }
            },
            "required": ["id"],
            "additionalProperties": false
        }`),
	}, r.deleteTask)
}
