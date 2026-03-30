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
	"github.com/Ci7amka/metachat/internal/middleware"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting auth service")

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

	jwtSecret := getEnv("JWT_SECRET", "change-me-in-production")
	jwtMgr := middleware.NewJWTManager(jwtSecret)

	repo := NewRepository(db)
	svc := NewService(repo, jwtMgr)
	handler := NewHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", handler.Register)
	mux.HandleFunc("POST /login", handler.Login)
	mux.HandleFunc("POST /refresh", handler.Refresh)
	mux.HandleFunc("POST /logout", handler.Logout)
	mux.HandleFunc("GET /profile", handler.GetProfile)
	mux.HandleFunc("PUT /profile", handler.UpdateProfile)
	mux.HandleFunc("GET /user-profile", handler.GetUserProfile)
	mux.HandleFunc("POST /validate-token", handler.ValidateToken)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	port := getEnv("AUTH_PORT", "50051")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("auth service listening", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down auth service")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("auth service stopped")
}
