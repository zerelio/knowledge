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
	"github.com/zerelio/zerelio/internal/change"
	"github.com/zerelio/zerelio/internal/discovery"
	"github.com/zerelio/zerelio/internal/docs"
	"github.com/zerelio/zerelio/internal/embedding"
	"github.com/zerelio/zerelio/internal/events"
	"github.com/zerelio/zerelio/internal/graph"
	"github.com/zerelio/zerelio/internal/knowledge"
	"github.com/zerelio/zerelio/internal/llm"
	"github.com/zerelio/zerelio/internal/mcp"
	"github.com/zerelio/zerelio/internal/oidcauth"
	"github.com/zerelio/zerelio/internal/ratelimit"
	"github.com/zerelio/zerelio/internal/scheduler"
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

	// OTel trace + Prometheus metrics.
	otelProviders, err := telemetry.Setup(ctx, "zerelio", version, logger)
	if err != nil {
		logger.Warn("OTel setup failed — observability disabled", "error", err)
	}
	if otelProviders != nil {
		defer otelProviders.Shutdown(context.Background())
	}

	// OIDC SSO — optional; starts up gracefully if not configured.
	var oidcProvider *oidcauth.Provider
	oidcCfg := oidcauth.LoadConfig()
	if oidcCfg != nil {
		oidcProvider, err = oidcauth.NewProvider(ctx, oidcCfg)
		if err != nil {
			logger.Warn("OIDC provider init failed — SSO disabled", "error", err)
			oidcCfg = nil
		} else {
			logger.Info("OIDC SSO enabled", "issuer", oidcCfg.Issuer)
		}
	} else {
		logger.Info("OIDC SSO not configured (set ZERELIO_OIDC_ISSUER to enable)")
	}

	// Wire user-ID extractor for rate limiter (after JWT service is ready).
	ratelimit.SetUserIDExtractor(func(r *http.Request) string {
		if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
			return claims.UserID
		}
		return ""
	})

	backend, err := connectStorage(ctx, logger)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer backend.Close()

	// Run schema migrations automatically on startup (idempotent).
	if pgb, ok := backend.(*storage.PGBackend); ok {
		if err := storage.Migrate(ctx, pgb, logger); err != nil {
			return fmt.Errorf("schema migration: %w", err)
		}
	}

	orch := discovery.NewOrchestrator(backend, logger)
	loadPlugins(orch, logger)

	detector := change.New(backend, logger)
	orch.SetDetector(detector)

	bus := events.New(backend, logger)
	orch.SetEventBus(bus)

	var graphSvc *graph.Graph
	var searchSvc *search.Searcher
	var embedSvc *embedding.Embedder
	var docGen *docs.Generator

	// LLM provider — configured from env, noop if not set.
	// Wrapped with OTel tracing so every Embed/Complete call gets a span.
	llmProvider := llm.WithTracing(llm.NewFromEnv())
	if !llm.IsNoop(llmProvider) {
		logger.Info("LLM provider active", "provider", llmProvider.Name())
	} else {
		logger.Info("LLM provider not configured — AI features disabled (set ZERELIO_LLM_PROVIDER)")
	}

	// Knowledge summarizer — always created; LLM is optional (falls back to stats).
	summarizer := knowledge.New(backend, llmProvider, logger)
	orch.SetSummarizer(summarizer)

	if pgb, ok := backend.(*storage.PGBackend); ok {
		graphSvc = graph.New(pgb.Pool(), logger)
		if err := graphSvc.Setup(ctx); err != nil {
			logger.Warn("AGE graph setup failed — graph features disabled", "error", err)
			graphSvc = nil
		} else {
			orch.SetGraph(graphSvc)
		}
		searchSvc = search.New(pgb.Pool(), logger)
		searchSvc.SetLLM(llmProvider)
		embedSvc = embedding.New(llmProvider, pgb.Pool(), logger)
		docGen = docs.New(llmProvider, backend, graphSvc)
		orch.SetEmbedder(embedSvc)
	}

	// Discovery scheduler — starts cron-based automatic scope runs.
	sched := scheduler.New(backend, &orchestratorRunner{orch}, logger)
	if os.Getenv("ZERELIO_SCHEDULER_ENABLED") == "true" {
		go sched.Start(ctx)
	} else {
		logger.Info("scheduler disabled — set ZERELIO_SCHEDULER_ENABLED=true to enable")
	}

	handler := api.NewHandler(backend, jwtSvc, orch, logger)
	if graphSvc != nil {
		handler.SetGraph(graphSvc)
	}
	if searchSvc != nil {
		handler.SetSearcher(searchSvc)
	}
	if oidcProvider != nil {
		handler.SetOIDC(oidcProvider, oidcCfg)
	}
	if docGen != nil {
		handler.SetDocGenerator(docGen)
	}
	handler.SetEventBus(bus)
	mcpServer := mcp.NewServer(backend, logger)
	if graphSvc != nil {
		mcpServer.SetGraph(graphSvc)
	}
	if searchSvc != nil {
		mcpServer.SetSearcher(searchSvc)
	}

	r := chi.NewRouter()

	// Rate limiting — IP-level before everything.
	rlCfg := ratelimit.Config{}
	ipLimiter, userLimiter := ratelimit.Middleware(rlCfg, logger)

	r.Use(ipLimiter)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(telemetry.RequestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))
	r.Use(telemetry.AuditLogger(backend, logger))

	r.Get("/health", telemetry.HealthCheck(version))
	r.Get("/metrics", telemetry.MetricsHandler().ServeHTTP)
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, version)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", handler.HandleLogin)
		r.Post("/auth/register", handler.HandleRegister)
		r.Post("/auth/refresh", handler.HandleRefresh)
		// OIDC SSO endpoints — unauthenticated.
		r.Get("/auth/oidc/status", handler.HandleOIDCStatus)
		r.Get("/auth/oidc/authorize", handler.HandleOIDCAuthorize)
		r.Get("/auth/oidc/callback", handler.HandleOIDCCallback)
		// Health — unauthenticated (used by LBs + console dashboard).
		r.Get("/health", handler.HandleHealth)

		if authDisabled {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					claims := &auth.Claims{UserID: "dev_admin", Email: "admin@localhost", Roles: []string{"admin"}, TenantID: "dev"}
					ctx := auth.ContextWithClaims(r.Context(), claims)
					ctx = storage.WithTenantID(ctx, claims.TenantID)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})
		} else {
			r.Use(auth.Authenticator(jwtSvc))
		}
		r.Use(userLimiter)

		r.Get("/resources", handler.HandleListResources)
		r.Get("/resources/{id}", handler.HandleGetResource)
		r.Get("/search", handler.HandleSearch)
		r.Get("/graph/export", handler.HandleGraphExport)
		r.Get("/graph/neighborhood/{id}", handler.HandleNeighborhood)
		r.Post("/graph/cypher", handler.HandleCypher)

		r.Get("/docs/{id}", handler.HandleGenerateDoc)
		r.Post("/runbooks", handler.HandleGenerateRunbook)

		r.Get("/generations", handler.HandleListGenerations)
		r.Get("/generations/{id}", handler.HandleGetGeneration)
		r.Post("/generations/{id}/feedback", handler.HandleSubmitFeedback)
		r.Put("/generations/{id}/status", handler.HandleDocumentStatus)

		r.Get("/discovery/scopes", handler.HandleListScopes)
		r.Get("/discovery/scopes/{id}", handler.HandleGetScope)
		r.Get("/discovery/scopes/{id}/runs", handler.HandleListRuns)
			r.Get("/discovery/runs", handler.HandleListAllRuns)
				r.Get("/discovery/runs/{runID}", handler.HandleGetRun)
		r.Get("/compliance/findings", handler.HandleListFindings)
			r.Patch("/compliance/findings/{id}", handler.HandleUpdateFindingStatus)
			r.Get("/discovery/scopes/{id}/changes", handler.HandleListChanges)
			r.Get("/knowledge/summaries", handler.HandleListKnowledgeSummaries)
		r.Get("/discovery/plugins", handler.HandleListPlugins)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("editor", "admin"))
			r.Post("/resources", handler.HandleCreateResources)
			r.Delete("/resources/{id}", handler.HandleDeleteResource)
			r.Post("/discovery/scopes", handler.HandleCreateScope)
			r.Patch("/discovery/scopes/{id}", handler.HandleUpdateScope)
			r.Delete("/discovery/scopes/{id}", handler.HandleDeleteScope)
			r.Post("/discovery/scopes/{id}/run", handler.HandleRunScope)
			r.Get("/webhooks", handler.HandleListWebhooks)
			r.Post("/webhooks", handler.HandleCreateWebhook)
			r.Delete("/webhooks/{id}", handler.HandleDeleteWebhook)
		})

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Get("/admin/users", handler.HandleListUsers)
			r.Post("/admin/users", handler.HandleCreateUser)
			r.Get("/admin/users/{id}", handler.HandleGetUser)
			r.Patch("/admin/users/{id}", handler.HandleUpdateUser)
			r.Delete("/admin/users/{id}", handler.HandleDeleteUser)
			r.Get("/admin/audit", handler.HandleListAuditEvents)
			r.Get("/admin/quality", handler.HandleQualityMetrics)
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

	// Background embedding sweep — catches resources not embedded after discovery.
	if embedSvc != nil {
		go embedSvc.RunBackground(ctx, 15*time.Minute)
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

// orchestratorRunner adapts *discovery.Orchestrator to scheduler.ScopeRunner.
// The scheduler interface returns only error; the orchestrator returns a run record too.
type orchestratorRunner struct{ o *discovery.Orchestrator }

func (r *orchestratorRunner) RunScope(ctx context.Context, scopeID string) error {
	_, err := r.o.RunScope(ctx, scopeID)
	return err
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
