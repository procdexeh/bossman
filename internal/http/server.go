package http

import (
	"fmt"
	"log/slog"
	gohttp "net/http"

	"procdexeh/bossman/internal/db"

	"github.com/jmoiron/sqlx"
)

const PORT = ":6969"

func Run(conn *sqlx.DB) {
	gohttp.HandleFunc("/", func(w gohttp.ResponseWriter, r *gohttp.Request) {
		fmt.Println("HELLO HTTP SERVER")
		w.WriteHeader(gohttp.StatusOK)
		fmt.Fprint(w, "hello")
	})

	gohttp.HandleFunc("/health", func(w gohttp.ResponseWriter, r *gohttp.Request) {
		slog.Info("HEALTH CHECK", "FROM", r.RemoteAddr)
		w.WriteHeader(gohttp.StatusOK)
		fmt.Fprint(w, "ok")
	})

	gohttp.HandleFunc("POST /task", func(w gohttp.ResponseWriter, r *gohttp.Request) {
		task := &db.Task{
			ID:          db.NewTaskID(),
			Description: r.RemoteAddr,
		}
		err := db.InsertTask(conn, task)
		if err != nil {
			slog.Error("HTTP SERVER ERROR", slog.Any("error", err))
			w.WriteHeader(gohttp.StatusInternalServerError)
			fmt.Fprint(w, "internal server error.")
			return
		}
		w.WriteHeader(gohttp.StatusOK)
		fmt.Fprint(w, "ok")
	})

	slog.Info("LISTENING ON", "PORT", PORT)
	err := gohttp.ListenAndServe(PORT, nil)
	if err != nil {
		slog.Error("HTTP SERVER ERROR", slog.Any("error", err))
	}
}
