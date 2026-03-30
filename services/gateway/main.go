package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Ci7amka/metachat/internal/middleware"
	"github.com/Ci7amka/metachat/internal/models"
	"github.com/gorilla/websocket"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type Gateway struct {
	authURL      string
	messagingURL string
	jwtMgr       *middleware.JWTManager
	httpClient   *http.Client
}

func NewGateway(authURL, messagingURL string, jwtMgr *middleware.JWTManager) *Gateway {
	return &Gateway{
		authURL:      authURL,
		messagingURL: messagingURL,
		jwtMgr:       jwtMgr,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// proxyRequest forwards an HTTP request to a backend service.
func (g *Gateway) proxyRequest(w http.ResponseWriter, r *http.Request, targetURL, method, path string, addUserID bool) {
	var bodyReader io.Reader
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	url := targetURL + path
	// Forward query parameters
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), method, url, bodyReader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create proxy request")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if addUserID {
		userID := middleware.GetUserID(r.Context())
		if userID != "" {
			req.Header.Set("X-User-ID", userID)
		}
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error("proxy request failed", "url", url, "error", err)
		writeError(w, http.StatusBadGateway, "service unavailable")
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleWebSocket upgrades the connection and proxies to messaging service.
func (g *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Connect to messaging service WebSocket
	wsURL := strings.Replace(g.messagingURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += "/ws"

	header := http.Header{}
	header.Set("X-User-ID", userID)

	backendConn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		slog.Error("failed to connect to messaging ws", "error", err)
		writeError(w, http.StatusBadGateway, "messaging service unavailable")
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("failed to upgrade ws", "error", err)
		backendConn.Close()
		return
	}

	// Bidirectional proxy
	go func() {
		defer clientConn.Close()
		defer backendConn.Close()
		for {
			msgType, msg, err := backendConn.ReadMessage()
			if err != nil {
				return
			}
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	go func() {
		defer clientConn.Close()
		defer backendConn.Close()
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if err := backendConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, models.ErrorResponse{Error: msg})
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("starting gateway service")

	jwtSecret := getEnv("JWT_SECRET", "change-me-in-production")
	jwtMgr := middleware.NewJWTManager(jwtSecret)

	authURL := getEnv("AUTH_SERVICE_URL", "http://localhost:50051")
	messagingURL := getEnv("MESSAGING_SERVICE_URL", "http://localhost:50052")
	rateRPS := 10.0
	rateBurst := 20.0

	gw := NewGateway(authURL, messagingURL, jwtMgr)
	rateLimiter := middleware.NewRateLimiter(rateRPS, rateBurst)

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// --- Public routes (no auth) ---
	mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, authURL, "POST", "/register", false)
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, authURL, "POST", "/login", false)
	})
	mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, authURL, "POST", "/refresh", false)
	})

	// --- Protected routes (require auth) ---
	authMW := middleware.AuthMiddleware(jwtMgr)

	// Auth service routes
	protectedMux := http.NewServeMux()

	protectedMux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, authURL, "POST", "/logout", true)
	})
	protectedMux.HandleFunc("GET /api/v1/profile", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, authURL, "GET", "/profile", true)
	})
	protectedMux.HandleFunc("PUT /api/v1/profile", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, authURL, "PUT", "/profile", true)
	})
	protectedMux.HandleFunc("GET /api/v1/users/profile", func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("id")
		if userID == "" {
			writeError(w, http.StatusBadRequest, "user id is required")
			return
		}
		gw.proxyRequest(w, r, authURL, "GET", fmt.Sprintf("/user-profile?user_id=%s", userID), true)
	})

	// Messaging service routes
	protectedMux.HandleFunc("GET /api/v1/conversations", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, messagingURL, "GET", "/conversations", true)
	})
	protectedMux.HandleFunc("POST /api/v1/conversations", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, messagingURL, "POST", "/conversations", true)
	})
	protectedMux.HandleFunc("GET /api/v1/conversations/messages", func(w http.ResponseWriter, r *http.Request) {
		convID := r.URL.Query().Get("conversation_id")
		limit := r.URL.Query().Get("limit")
		before := r.URL.Query().Get("before")
		query := fmt.Sprintf("conversation_id=%s", convID)
		if limit != "" {
			query += "&limit=" + limit
		}
		if before != "" {
			query += "&before=" + before
		}
		gw.proxyRequest(w, r, messagingURL, "GET", "/messages?"+query, true)
	})
	protectedMux.HandleFunc("POST /api/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, messagingURL, "POST", "/messages", true)
	})
	protectedMux.HandleFunc("POST /api/v1/messages/read", func(w http.ResponseWriter, r *http.Request) {
		gw.proxyRequest(w, r, messagingURL, "POST", "/messages/read", true)
	})

	// WebSocket
	protectedMux.HandleFunc("GET /api/v1/ws", gw.handleWebSocket)

	// Mount protected routes under auth middleware
	mux.Handle("POST /api/v1/auth/logout", authMW(protectedMux))
	mux.Handle("GET /api/v1/profile", authMW(protectedMux))
	mux.Handle("PUT /api/v1/profile", authMW(protectedMux))
	mux.Handle("GET /api/v1/users/profile", authMW(protectedMux))
	mux.Handle("GET /api/v1/conversations", authMW(protectedMux))
	mux.Handle("POST /api/v1/conversations", authMW(protectedMux))
	mux.Handle("GET /api/v1/conversations/messages", authMW(protectedMux))
	mux.Handle("POST /api/v1/messages", authMW(protectedMux))
	mux.Handle("POST /api/v1/messages/read", authMW(protectedMux))
	mux.Handle("GET /api/v1/ws", authMW(protectedMux))

	// Apply global middleware
	var handler http.Handler = mux
	handler = middleware.RateLimitMiddleware(rateLimiter)(handler)
	handler = middleware.CORSMiddleware(handler)
	handler = middleware.LoggingMiddleware(handler)

	port := getEnv("GATEWAY_PORT", "8080")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("gateway listening", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gateway")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("gateway stopped")
}
