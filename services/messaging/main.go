package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ci7amka/metachat/internal/database"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting messaging service")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.NewPostgresPool(ctx, database.Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     getEnv("POSTGRES_PORT", "5432"),
		User:     getEnv("POSTGRES_USER", "metachat"),
		Password: getEnv("POSTGRES_PASSWORD", "metachat_secret"),
		DBName:   getEnv("POSTGRES_DB", "metachat"),
	})
	if err != nil {
		slog.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	hub := NewHub()
	repo := NewRepository(db)
	svc := NewService(repo, hub)
	handler := NewHandler(svc, hub)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", handler.HandleWebSocket)
	mux.HandleFunc("GET /conversations", handler.GetConversations)
	mux.HandleFunc("POST /conversations", handler.CreateConversation)
	mux.HandleFunc("GET /messages", handler.GetMessages)
	mux.HandleFunc("POST /messages", handler.SendMessage)
	mux.HandleFunc("POST /messages/read", handler.MarkRead)
	mux.HandleFunc("GET /online", handler.GetOnlineUsers)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	port := getEnv("MESSAGING_PORT", "50052")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("messaging service listening", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down messaging service")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("messaging service stopped")
}
