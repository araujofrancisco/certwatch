package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/araujofrancisco/certwatch/internal/auth"
)

func TestLogging(t *testing.T) {
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRecovery(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))
	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	handler := CORS([]string{"http://localhost:8080"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" {
		t.Error("expected CORS header")
	}
}

func TestAuthMiddleware(t *testing.T) {
	authenticator := auth.New("test-secret", time.Hour)
	token, _ := authenticator.GenerateToken(1, "user@example.com")

	mw := Auth(authenticator)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(UserClaimsKey).(*auth.Claims)
		if claims.UserID != 1 {
			t.Errorf("expected user 1, got %d", claims.UserID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/domains", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	authenticator := auth.New("test-secret", time.Hour)
	mw := Auth(authenticator)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/domains", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()

	if !rl.Allow("test-ip") {
		t.Error("expected first request allowed")
	}
	if !rl.Allow("test-ip") {
		t.Error("expected second request allowed")
	}
	if !rl.Allow("test-ip") {
		t.Error("expected third request allowed")
	}
	if rl.Allow("test-ip") {
		t.Error("expected fourth request blocked")
	}
	// Different IP should be allowed
	if !rl.Allow("other-ip") {
		t.Error("expected different IP allowed")
	}
}

func TestRateLimiter_Stop(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	rl.Stop()
	// Should not panic on subsequent calls
	rl.Stop()
}

func TestRateLimiter_Expires(t *testing.T) {
	// Use a very short window so the entry expires
	rl := NewRateLimiter(1, 50*time.Millisecond)
	defer rl.Stop()

	if !rl.Allow("test") {
		t.Error("expected allowed")
	}
	if rl.Allow("test") {
		t.Error("expected blocked within window")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.Allow("test") {
		t.Error("expected allowed after expiry")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remote   string
		expected string
	}{
		{"x-forwarded-for", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "5.6.7.8:1234", "1.2.3.4"},
		{"x-forwarded-for-multi", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"}, "9.9.9.9:99", "1.2.3.4"},
		{"x-real-ip", map[string]string{"X-Real-IP": "4.3.2.1"}, "5.6.7.8:1234", "4.3.2.1"},
		{"remote-addr-no-port", nil, "1.2.3.4", "1.2.3.4"},
		{"remote-addr-with-port", nil, "1.2.3.4:8080", "1.2.3.4"},
		{"x-forwarded-for-wins", map[string]string{"X-Forwarded-For": "1.1.1.1", "X-Real-IP": "2.2.2.2"}, "3.3.3.3:80", "1.1.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			ip := clientIP(req)
			if ip != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, ip)
			}
		})
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	authenticator := auth.New("test-secret", time.Hour)
	mw := Auth(authenticator)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/domains", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
