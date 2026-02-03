package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

const PORT = ":6969"

func main() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("HELLO HTTP SERVER")
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("HEALTH CHECK", "FROM", r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	slog.Info("LISTENING ON", "PORT", PORT)
	err := http.ListenAndServe(PORT, nil)
	if err != nil {
		slog.Error("HTTP SERVER ERROR", slog.Any("error", err))
		os.Exit(1)
	}

}

