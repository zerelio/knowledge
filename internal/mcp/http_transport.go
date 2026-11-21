package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// HTTPTransport implements MCP over HTTP + Server-Sent Events.
type HTTPTransport struct {
	handler  MessageHandler
	sessions sync.Map
	logger   *slog.Logger
	server   *http.Server
	port     int
	nextID   atomic.Int64
}

type sseSession struct {
	id      string
	channel chan []byte
	cancel  context.CancelFunc
}

func NewHTTPTransport(handler MessageHandler, port int, logger *slog.Logger) *HTTPTransport {
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPTransport{handler: handler, port: port, logger: logger}
}

func (t *HTTPTransport) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", t.handleSSE)
	mux.HandleFunc("/message", t.handleMessage)

	t.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", t.port),
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		t.logger.Info("mcp http transport started", "port", t.port)
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return t.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (t *HTTPTransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sessionID := fmt.Sprintf("sess-%d", t.nextID.Add(1))
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	session := &sseSession{id: sessionID, channel: make(chan []byte, 256), cancel: cancel}
	t.sessions.Store(sessionID, session)
	defer t.sessions.Delete(sessionID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "event: endpoint\ndata: /message?session_id=%s\n\n", sessionID)
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-session.channel:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(msg)) //nolint:gosec // msg is server-generated JSON, not user input
			flusher.Flush()
		}
	}
}

func (t *HTTPTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeJSONError(w, ErrInvalidParams, "missing session_id", http.StatusBadRequest)
		return
	}

	val, ok := t.sessions.Load(sessionID)
	if !ok {
		writeJSONError(w, ErrInvalidParams, "invalid or expired session_id", http.StatusBadRequest)
		return
	}
	session := val.(*sseSession)

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		writeJSONError(w, ErrParseError, "empty or unreadable body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	response, err := t.handler.HandleMessage(r.Context(), body)
	if err != nil {
		writeJSONError(w, ErrInternalError, "internal error", http.StatusInternalServerError)
		return
	}

	if response != nil {
		select {
		case session.channel <- response:
		default:
			t.logger.Warn("sse channel full, dropping message", "session_id", sessionID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"accepted","session_id":%q}`, sessionID)
}

func (t *HTTPTransport) BroadcastNotification(method string, params interface{}) error {
	notif := map[string]interface{}{"jsonrpc": JSONRPCVersion, "method": method}
	if params != nil {
		notif["params"] = params
	}
	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}
	t.sessions.Range(func(_, value interface{}) bool {
		s := value.(*sseSession)
		select {
		case s.channel <- data:
		default:
		}
		return true
	})
	return nil
}

func writeJSONError(w http.ResponseWriter, code int, message string, httpStatus int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	resp := Response{JSONRPC: JSONRPCVersion, ID: json.RawMessage("null"), Error: &RPCError{Code: code, Message: message}}
	json.NewEncoder(w).Encode(resp)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
