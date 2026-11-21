package telemetry

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/zerelio/knowledge/internal/auth"
	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

// AuditLogger returns middleware that records mutation requests (POST, PUT, PATCH, DELETE)
// to the audit_events table after the response is written.
// It is a no-op for GET/HEAD and for auth endpoints.
func AuditLogger(store storage.Backend, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit mutations.
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)

			// Skip auth routes — login/register/refresh are audited separately.
			path := r.URL.Path
			if strings.Contains(path, "/auth/") {
				return
			}

			claims := auth.ClaimsFromContext(r.Context())
			userID, tenantID := "", ""
			if claims != nil {
				userID = claims.UserID
				tenantID = claims.TenantID
			}

			event := &types.AuditEvent{
				ID:         fmt.Sprintf("aud_%d", start.UnixNano()),
				UserID:     userID,
				TenantID:   tenantID,
				Action:     r.Method + " " + path,
				IP:         realIP(r),
				StatusCode: ww.Status(),
				CreatedAt:  start,
			}

			// Best-effort — don't fail the request if audit logging fails.
			if err := store.LogAuditEvent(r.Context(), event); err != nil {
				logger.Warn("audit log failed", "action", event.Action, "error", err)
			}
		})
	}
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		for i := range ip {
			if ip[i] == ',' {
				return ip[:i]
			}
		}
		return ip
	}
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
