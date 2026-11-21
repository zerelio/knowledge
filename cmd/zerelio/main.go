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
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/zerelio/zerelio/internal/api"
	"github.com/zerelio/zerelio/internal/auth"
	"github.com/zerelio/zerelio/internal/discovery"
	"github.com/zerelio/zerelio/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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

	jwtSvc, err := loadJWTService(authDisabled)
	if err != nil {
		return fmt.Errorf("jwt service: %w", err)
	}

	backend := storage.NewMemoryBackend()
	if err := backend.Connect(ctx); err != nil {
		return fmt.Errorf("storage connect: %w", err)
	}
	defer backend.Close()

	orch := discovery.NewOrchestrator(backend, logger)
	handler := api.NewHandler(backend, jwtSvc, orch, logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, "0.1.0")
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/login", handler.HandleLogin)
		r.Post("/register", handler.HandleRegister)

		if authDisabled {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					claims := &auth.Claims{
						UserID: "dev_admin",
						Email:  "admin@localhost",
						Roles:  []string{"admin"},
					}
					ctx := auth.ContextWithClaims(r.Context(), claims)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})
		} else {
			r.Use(auth.Authenticator(jwtSvc))
		}

		r.Get("/resources", handler.HandleListResources)
		r.Get("/resources/{id}", handler.HandleGetResource)
		r.Get("/graph/export", handler.HandleGraphExport)

		r.Get("/discovery/scopes", handler.HandleListScopes)
		r.Get("/discovery/scopes/{id}/runs", handler.HandleListRuns)

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

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting zerelio server", "addr", addr, "auth_disabled", authDisabled)
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
		slog.Warn("auth disabled: using ephemeral JWT keys (development mode only)")
		return auth.NewJWTService(privKey, 15*time.Minute, 7*24*time.Hour), nil
	}

	return nil, fmt.Errorf("ZERELIO_JWT_PRIVATE_KEY is required (set ZERELIO_AUTH_DISABLED=true for development)")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
