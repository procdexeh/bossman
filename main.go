package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rs/xid"
	_ "modernc.org/sqlite"
)

var db *sqlx.DB

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

func initDB(path string) error {
	var err error
	db, err = sqlx.Connect("sqlite", path)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(
		context.Background(),
		`CREATE TABLE IF NOT EXISTS tasks (
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
`)
	return err
}

func InsertTask(t *Task) error {
	_, err := db.NamedExecContext(
		context.Background(),
		`INSERT INTO tasks (id, description) VALUES (:id, :description)`,
		t,
	)
	slog.Info("DATABASE INSERT", "task", t)
	return err
}

func QueryTasks() ([]Task, error) {
	var tasks []Task
	err := db.SelectContext(context.Background(), &tasks, `SELECT * FROM tasks`)
	return tasks, err
}

const PORT = ":6969"

func main() {
	id := 1

	err := initDB("./bossman.db")
	if err != nil {
		slog.Error("DATABASE INIT ERROR", slog.Any("error", err))
		os.Exit(1)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("HELLO HTTP SERVER")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("HEALTH CHECK", "FROM", r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	http.HandleFunc("POST /task", func(w http.ResponseWriter, r *http.Request) {
		task := &Task{
			ID:          "task_" + xid.New().String(),
			Description: r.RemoteAddr,
		}
		err := InsertTask(task)
		if err != nil {
			slog.Error("HTTP SERVER ERROR", slog.Any("error", err))
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "internal server error.")
			return
		}
		id++
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	slog.Info("LISTENING ON", "PORT", PORT)
	err = http.ListenAndServe(PORT, nil)
	if err != nil {
		slog.Error("HTTP SERVER ERROR", slog.Any("error", err))
		os.Exit(1)
	}
}
