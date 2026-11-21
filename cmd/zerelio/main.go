package main

import (
	"context"
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/zerelio/zerelio/internal/api"
	"github.com/zerelio/zerelio/internal/auth"
	"github.com/zerelio/zerelio/internal/discovery"
	"github.com/zerelio/zerelio/internal/graph"
	"github.com/zerelio/zerelio/internal/mcp"
	"github.com/zerelio/zerelio/internal/search"
	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/internal/telemetry"
)

const version = "0.1.0"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	addr := envOr("ZERELIO_ADDR", ":8080")
	authDisabled := os.Getenv("ZERELIO_AUTH_DISABLED") == "true"
	mcpHTTPEnabled := os.Getenv("ZERELIO_MCP_HTTP") == "true"
	mcpStdioEnabled := os.Getenv("ZERELIO_MCP_STDIO") == "true"
	mcpHTTPPort := envOrInt("ZERELIO_MCP_PORT", 9090)

	jwtSvc, err := loadJWTService(authDisabled)
	if err != nil {
		return fmt.Errorf("jwt service: %w", err)
	}

	backend, err := connectStorage(ctx, logger)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer backend.Close()

	orch := discovery.NewOrchestrator(backend, logger)
	loadPlugins(orch, logger)

	var graphSvc *graph.Graph
	var searchSvc *search.Searcher
	if pgb, ok := backend.(*storage.PGBackend); ok {
		graphSvc = graph.New(pgb.Pool(), logger)
		if err := graphSvc.Setup(ctx); err != nil {
			logger.Warn("AGE graph setup failed — graph features disabled", "error", err)
			graphSvc = nil
		} else {
			orch.SetGraph(graphSvc)
		}
		searchSvc = search.New(pgb.Pool(), logger)
	}

	handler := api.NewHandler(backend, jwtSvc, orch, logger)
	if graphSvc != nil {
		handler.SetGraph(graphSvc)
	}
	if searchSvc != nil {
		handler.SetSearcher(searchSvc)
	}
	mcpServer := mcp.NewServer(backend, logger)
	if graphSvc != nil {
		mcpServer.SetGraph(graphSvc)
	}
	if searchSvc != nil {
		mcpServer.SetSearcher(searchSvc)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(telemetry.RequestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	r.Get("/health", telemetry.HealthCheck(version))
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, version)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", handler.HandleLogin)
		r.Post("/auth/register", handler.HandleRegister)
		r.Post("/auth/refresh", handler.HandleRefresh)

		if authDisabled {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					claims := &auth.Claims{UserID: "dev_admin", Email: "admin@localhost", Roles: []string{"admin"}}
					ctx := auth.ContextWithClaims(r.Context(), claims)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})
		} else {
			r.Use(auth.Authenticator(jwtSvc))
		}

		r.Get("/resources", handler.HandleListResources)
		r.Get("/resources/{id}", handler.HandleGetResource)
		r.Get("/search", handler.HandleSearch)
		r.Get("/graph/export", handler.HandleGraphExport)
		r.Get("/graph/neighborhood/{id}", handler.HandleNeighborhood)
		r.Post("/graph/cypher", handler.HandleCypher)

		r.Get("/discovery/scopes", handler.HandleListScopes)
		r.Get("/discovery/scopes/{id}/runs", handler.HandleListRuns)
		r.Get("/discovery/plugins", handler.HandleListPlugins)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("editor", "admin"))
			r.Post("/resources", handler.HandleCreateResources)
			r.Delete("/resources/{id}", handler.HandleDeleteResource)
			r.Post("/discovery/scopes", handler.HandleCreateScope)
			r.Post("/discovery/scopes/{id}/run", handler.HandleRunScope)
		})

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Get("/admin/users", handler.HandleListUsers)
			r.Post("/admin/users", handler.HandleCreateUser)
		})
	})

	// MCP stdio transport — for direct AI agent integration.
	if mcpStdioEnabled {
		go func() {
			if err := mcpServer.ServeStdio(ctx); err != nil && err != context.Canceled {
				logger.Error("mcp stdio error", "error", err)
			}
		}()
	}

	// MCP HTTP+SSE transport — for web chat integrations.
	if mcpHTTPEnabled {
		go func() {
			logger.Info("starting mcp http server", "port", mcpHTTPPort)
			if err := mcpServer.ServeHTTP(ctx, mcpHTTPPort); err != nil && err != context.Canceled {
				logger.Error("mcp http error", "error", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting zerelio server", "addr", addr, "auth_disabled", authDisabled, "version", version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down gracefully")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}

func connectStorage(ctx context.Context, logger *slog.Logger) (storage.Backend, error) {
	dsn := os.Getenv("ZERELIO_POSTGRES_DSN")
	if dsn != "" {
		logger.Info("connecting to postgresql", "dsn_prefix", dsn[:min(len(dsn), 30)])
		pg := storage.NewPGBackend(dsn)
		if err := pg.Connect(ctx); err != nil {
			return nil, fmt.Errorf("postgres connect: %w", err)
		}
		logger.Info("postgresql connected")
		return pg, nil
	}
	logger.Warn("no ZERELIO_POSTGRES_DSN — using in-memory storage (data not persisted)")
	mem := storage.NewMemoryBackend()
	if err := mem.Connect(ctx); err != nil {
		return nil, err
	}
	return mem, nil
}

func loadJWTService(authDisabled bool) (*auth.JWTService, error) {
	keyPath := os.Getenv("ZERELIO_JWT_PRIVATE_KEY")
	if keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read private key: %w", err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("invalid PEM data in %s", keyPath)
		}
		if len(block.Bytes) != ed25519.SeedSize {
			return nil, fmt.Errorf("expected Ed25519 seed (%d bytes), got %d", ed25519.SeedSize, len(block.Bytes))
		}
		privKey := ed25519.NewKeyFromSeed(block.Bytes)
		return auth.NewJWTService(privKey, 15*time.Minute, 7*24*time.Hour), nil
	}
	if authDisabled {
		_, privKey, err := auth.GenerateKeyPair()
		if err != nil {
			return nil, err
		}
		slog.Warn("auth disabled: using ephemeral JWT keys (dev only)")
		return auth.NewJWTService(privKey, 15*time.Minute, 7*24*time.Hour), nil
	}
	return nil, fmt.Errorf("ZERELIO_JWT_PRIVATE_KEY required (or set ZERELIO_AUTH_DISABLED=true)")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func logLevel() slog.Level {
	switch os.Getenv("ZERELIO_LOG_LEVEL") {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// loadPlugins reads ZERELIO_PLUGIN_DIR and registers every executable it finds
// with the orchestrator as a subprocess plugin.
// Plugin names are derived from filenames: "plugin-aws" → "aws", "aws" → "aws".
// Set ZERELIO_PLUGIN_DIR=/path/to/plugins to enable.
func loadPlugins(orch *discovery.Orchestrator, logger *slog.Logger) {
	dir := os.Getenv("ZERELIO_PLUGIN_DIR")
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("cannot read ZERELIO_PLUGIN_DIR", "dir", dir, "error", err)
		return
	}

	loaded := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Skip non-executable files.
		if info.Mode()&0o111 == 0 {
			continue
		}

		path := filepath.Join(dir, e.Name())
		name := strings.TrimPrefix(e.Name(), "plugin-")
		name = strings.TrimSuffix(name, ".exe")

		if err := orch.LoadSubprocessPlugin(name, path); err != nil {
			logger.Error("failed to load plugin", "name", name, "path", path, "error", err)
			continue
		}
		logger.Info("loaded plugin", "name", name, "path", path)
		loaded++
	}

	if loaded > 0 {
		logger.Info("plugin loader complete", "count", loaded, "dir", dir)
	}
}
