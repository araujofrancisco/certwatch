package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/araujofrancisco/certwatch/internal/auth"
)

type contextKey string

const UserClaimsKey contextKey = "user_claims"

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start).String(),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "error", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Timeout enforces a hard deadline on every request. Handlers that honor
// request context (all service/repository paths once context propagation is
// in place) are cancelled when it fires; the client gets a 503.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "{\"error\":\"request timeout\"}\n")
	}
}

func Auth(authenticator *auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := r.Header.Get("Authorization")
			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}
			if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
				tokenStr = tokenStr[7:]
			}
			claims, err := authenticator.ValidateToken(tokenStr)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CORS(allowedOrigins []string, allowLocalhost bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := ""
			for _, o := range allowedOrigins {
				if o == "*" || sameOrigin(o, origin) {
					allowed = origin
					break
				}
			}
			if allowed == "" && allowLocalhost && isLocalhostOrigin(origin) {
				allowed = origin
			}
			if allowed != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowed)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sameOrigin(a, b string) bool {
	if a == b {
		return true
	}
	au, err := url.Parse(a)
	if err != nil {
		return false
	}
	bu, err := url.Parse(b)
	if err != nil {
		return false
	}
	return au.Hostname() == bu.Hostname() && au.Port() == bu.Port() && au.Scheme == bu.Scheme
}

func isLocalhostOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

type RateLimiter struct {
	mu       sync.Mutex
	entries  map[string][]time.Time
	limit    int
	window   time.Duration
	stop     chan struct{}
	closeOnce sync.Once
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
		stop:    make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Stop() {
	rl.closeOnce.Do(func() {
		close(rl.stop)
	})
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, times := range rl.entries {
				var kept []time.Time
				for _, t := range times {
					if now.Sub(t) < rl.window {
						kept = append(kept, t)
					}
				}
				if len(kept) == 0 {
					delete(rl.entries, key)
				} else {
					rl.entries[key] = kept
				}
			}
			rl.mu.Unlock()
		}
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	times := rl.entries[key]
	var recent []time.Time
	for _, t := range times {
		if now.Sub(t) < rl.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= rl.limit {
		rl.entries[key] = recent
		return false
	}
	rl.entries[key] = append(recent, now)
	return true
}

// ClientIP returns the best-effort client address for rate limiting. Proxy
// headers are only honored when trustProxy is true: they are attacker
// controlled on direct connections and would let clients rotate their identity
// to bypass the limiter. When trusted, the right-most X-Forwarded-For entry is
// used, since proxies append and client-supplied leftmost values can be forged.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
		if real := r.Header.Get("X-Real-IP"); real != "" {
			return strings.TrimSpace(real)
		}
	}
	ip := r.RemoteAddr
	if addr := strings.LastIndex(ip, ":"); addr != -1 {
		ip = ip[:addr]
	}
	return ip
}

func RateLimit(rl *RateLimiter, trustProxyHeaders bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r, trustProxyHeaders)
			if !rl.Allow(ip) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
