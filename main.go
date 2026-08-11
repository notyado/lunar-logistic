package main

import (
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"lunar-logistics/internal/game"
	"lunar-logistics/internal/server"
	"lunar-logistics/internal/store"
)

//go:embed static/* static/assets/*
var webFiles embed.FS

func main() {
	addr := env("ADDR", ":8080")
	dbPath := env("DB_PATH", "data/luna.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatal(err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	engine := game.New(st)
	srv := server.New(engine, server.StaticFS(webFiles))

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Lunar Logistics control server listening on %s", addr)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
