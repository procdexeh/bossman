package db

import (
	"context"
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/rs/xid"
	_ "modernc.org/sqlite"
)

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
`

func InitDB(path string) (*sqlx.DB, error) {
	conn, err := sqlx.Connect("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = conn.ExecContext(context.Background(), schema)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func NewTaskID() string {
	return "task_" + xid.New().String()
}

func InsertTask(conn *sqlx.DB, t *Task) error {
	_, err := conn.NamedExecContext(
		context.Background(),
		`INSERT INTO tasks (id, description) VALUES (:id, :description)`,
		t,
	)
	slog.Info("DATABASE INSERT", "task", t)
	return err
}

func QueryTasks(conn *sqlx.DB) ([]Task, error) {
	var tasks []Task
	err := conn.SelectContext(context.Background(), &tasks, `SELECT * FROM tasks`)
	return tasks, err
}
