// Package ratelimit provides HTTP middleware for token-bucket rate limiting.
// Two limits are applied in sequence:
//   - IP-level: before authentication, applied to all requests
//   - User-level: after authentication, keyed on user ID from JWT claims
//
// Stale entries are pruned every minute to bound memory usage.
package ratelimit

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Store holds per-key rate limiters with periodic cleanup.
type Store struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rps      rate.Limit
	burst    int
}

func newStore(rps float64, burst int) *Store {
	s := &Store{
		visitors: make(map[string]*visitor),
		rps:      rate.Limit(rps),
		burst:    burst,
	}
	go s.cleanup()
	return s
}

func (s *Store) allow(key string) bool {
	s.mu.Lock()
	v, ok := s.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(s.rps, s.burst)}
		s.visitors[key] = v
	}
	v.lastSeen = time.Now()
	allowed := v.limiter.Allow()
	s.mu.Unlock()
	return allowed
}

// cleanup removes visitors not seen in the last 3 minutes.
func (s *Store) cleanup() {
	for range time.Tick(time.Minute) {
		s.mu.Lock()
		cutoff := time.Now().Add(-3 * time.Minute)
		for key, v := range s.visitors {
			if v.lastSeen.Before(cutoff) {
				delete(s.visitors, key)
			}
		}
		s.mu.Unlock()
	}
}

// Config holds rate limit settings loaded from environment / options.
type Config struct {
	// IPRPS is requests per second allowed per source IP (default 100).
	IPRPS float64
	// IPBurst is the burst size per IP (default 50).
	IPBurst int
	// UserRPS is requests per second allowed per authenticated user (default 50).
	UserRPS float64
	// UserBurst is burst size per user (default 25).
	UserBurst int
}

func (c *Config) defaults() {
	if c.IPRPS <= 0 {
		c.IPRPS = 100
	}
	if c.IPBurst <= 0 {
		c.IPBurst = 50
	}
	if c.UserRPS <= 0 {
		c.UserRPS = 50
	}
	if c.UserBurst <= 0 {
		c.UserBurst = 25
	}
}

// Middleware returns two middlewares: IPLimiter and UserLimiter.
// IPLimiter must be applied before auth; UserLimiter after auth.
func Middleware(cfg Config, logger *slog.Logger) (ipLimiter, userLimiter func(http.Handler) http.Handler) {
	cfg.defaults()
	ipStore := newStore(cfg.IPRPS, cfg.IPBurst)
	userStore := newStore(cfg.UserRPS, cfg.UserBurst)

	ipLimiter = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if !ipStore.allow(ip) {
				logger.Warn("rate limit exceeded", "type", "ip", "ip", ip, "path", r.URL.Path)
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	userLimiter = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract user ID from context (set by auth.Authenticator).
			// If not present (unauthenticated route), skip user limit.
			uid := userIDFromContext(r)
			if uid != "" && !userStore.allow(uid) {
				logger.Warn("rate limit exceeded", "type", "user", "user_id", uid, "path", r.URL.Path)
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	return ipLimiter, userLimiter
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For may be a comma-separated list; use the first entry.
		for i := 0; i < len(ip); i++ {
			if ip[i] == ',' {
				return ip[:i]
			}
		}
		return ip
	}
	// Fall back to RemoteAddr, stripping port.
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// userIDFromContext pulls the user ID from the JWT claims stored by auth.Authenticator.
// We import by function name to avoid circular imports.
var userIDFromContext = func(r *http.Request) string { return "" }

// SetUserIDExtractor replaces the default no-op with the real extractor.
// Called from main after wiring auth.
func SetUserIDExtractor(fn func(*http.Request) string) {
	userIDFromContext = fn
}
