package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/xid"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    parent_id   TEXT REFERENCES tasks(id),
    description TEXT NOT NULL,
    context     TEXT NOT NULL DEFAULT '',
    priority    INTEGER NOT NULL DEFAULT 3
        CHECK (priority BETWEEN 1 AND 5),
    status      TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    result      TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    started_at  TEXT,
    completed_at TEXT,
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE TABLE IF NOT EXISTS task_blockers (
    task_id       TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_by_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, blocked_by_id),
    CHECK (task_id != blocked_by_id)
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
CREATE INDEX IF NOT EXISTS idx_task_blockers_task ON task_blockers(task_id);
CREATE INDEX IF NOT EXISTS idx_task_blockers_blocked_by ON task_blockers(blocked_by_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status_priority ON tasks(status, priority);
`

type Task struct {
	ID          string  `db:"id"`
	ParentID    *string `db:"parent_id"`
	Description string  `db:"description"`
	Context     string  `db:"context"`
	Priority    int     `db:"priority"`
	Status      string  `db:"status"`
	Result      *string `db:"result"`
	CreatedAt   string  `db:"created_at"`
	StartedAt   *string `db:"started_at"`
	CompletedAt *string `db:"completed_at"`
	UpdatedAt   string  `db:"updated_at"`
}

type ListOpts struct {
	Status   *string
	ParentID *string
	Limit    int
}

type UpdateOpts struct {
	Description *string
	Priority    *int
	Status      *string
	Context     *string
	Result      *string
}

func InitDB(path string) (*sqlx.DB, error) {
	conn, err := sqlx.Connect("sqlite",
		path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	if _, err = conn.ExecContext(context.Background(), schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return conn, nil
}

func NewTaskID() string {
	return "task_" + xid.New().String()
}

func InsertTask(ctx context.Context, db *sqlx.DB, t *Task) error {
	_, err := db.NamedExecContext(ctx,
		`INSERT INTO tasks (id, description, parent_id, priority, context)
         VALUES (:id, :description, :parent_id, :priority, :context)`,
		t,
	)
	return err
}

func QueryTasks(ctx context.Context, db *sqlx.DB, opts ListOpts) ([]Task, error) {
	query := "SELECT * FROM tasks WHERE 1=1"
	args := make(map[string]any)

	if opts.Status != nil {
		query += " AND status = :status"
		args["status"] = *opts.Status
	}

	if opts.ParentID != nil {
		query += " AND parent_id = :parent_id"
		args["parent_id"] = *opts.ParentID
	}

	query += " ORDER BY priority ASC, created_at DESC"

	if opts.Limit > 0 {
		query += " LIMIT :limit"
		args["limit"] = opts.Limit
	}

	var tasks []Task
	rows, err := db.NamedQueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t Task
		if err := rows.StructScan(&t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func GetTask(ctx context.Context, db *sqlx.DB, id string) (*Task, error) {
	var t Task
	err := db.GetContext(ctx, &t, "SELECT * FROM tasks WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func UpdateTask(ctx context.Context, db *sqlx.DB, id string, opts UpdateOpts) error {
	setClauses := []string{"updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')"}
	args := map[string]any{"id": id}

	if opts.Description != nil {
		setClauses = append(setClauses, "description = :description")
		args["description"] = *opts.Description
	}

	if opts.Priority != nil {
		setClauses = append(setClauses, "priority = :priority")
		args["priority"] = *opts.Priority
	}

	if opts.Status != nil {
		setClauses = append(setClauses, "status = :status")
		args["status"] = *opts.Status
	}

	if opts.Context != nil {
		setClauses = append(setClauses, "context = :context")
		args["context"] = *opts.Context
	}

	if opts.Result != nil {
		setClauses = append(setClauses, "result = :result")
		args["result"] = *opts.Result
	}

	query := "UPDATE tasks SET " + strings.Join(setClauses, ", ") + " WHERE id = :id"

	result, err := db.NamedExecContext(ctx, query, args)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func DeleteTask(ctx context.Context, db *sqlx.DB, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil

}

func TaskExists(ctx context.Context, db *sqlx.DB, id string) (bool, error) {
	var exists bool
	err := db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM tasks WHERE id = ?)", id)
	return exists, err
}

func AddBlocker(ctx context.Context, db *sqlx.DB, taskID, blockedByID string) error {
	_, err := db.ExecContext(ctx, "INSERT INTO task_blockers (task_id, blocked_by_id) VALUES (?, ?)",
		taskID, blockedByID)
	return err
}

func RemoveBlocker(ctx context.Context, db *sqlx.DB, taskID, blockedByID string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM task_blockers WHERE task_id = ? AND blocked_by_id = ?", taskID, blockedByID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}
func GetBlockers(ctx context.Context, db *sqlx.DB, taskID string) ([]Task, error) {
	var tasks []Task
	err := db.SelectContext(ctx, &tasks,
		`SELECT t.* from tasks t 
		 INNER JOIN task_blockers tb ON t.id = tb.blocked_by_id
		 WHERE tb.task_id = ?`, taskID)
	return tasks, err
}
