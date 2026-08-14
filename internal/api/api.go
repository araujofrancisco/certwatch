package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/middleware"
	"github.com/araujofrancisco/certwatch/internal/services"
)

const Version = "0.1.0"

type Handler struct {
	domains       *services.DomainService
	certs         *services.CertificateService
	authSvc       *services.AuthService
	authN         *auth.Authenticator
	db            *sql.DB
	rateLimiter   *middleware.RateLimiter
	readLimiter   *middleware.RateLimiter
}

func NewHandler(domains *services.DomainService, certs *services.CertificateService, authSvc *services.AuthService, authN *auth.Authenticator, db *sql.DB, rateLimiter, readLimiter *middleware.RateLimiter) *Handler {
	return &Handler{domains: domains, certs: certs, authSvc: authSvc, authN: authN, db: db, rateLimiter: rateLimiter, readLimiter: readLimiter}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.Auth(h.authN)
	rateLimit := middleware.RateLimit(h.rateLimiter)
	readLimit := middleware.RateLimit(h.readLimiter)

	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /api/version", h.version)

	mux.Handle("POST /api/auth/register", rateLimit(http.HandlerFunc(h.register)))
	mux.Handle("POST /api/auth/login", rateLimit(http.HandlerFunc(h.login)))
	mux.Handle("PUT /api/auth/password", rateLimit(authMiddleware(http.HandlerFunc(h.changePassword))))

	mux.Handle("GET /api/domains", readLimit(authMiddleware(http.HandlerFunc(h.listDomains))))
	mux.Handle("POST /api/domains/import", rateLimit(authMiddleware(http.HandlerFunc(h.importDomains))))
	mux.Handle("POST /api/domains", rateLimit(authMiddleware(http.HandlerFunc(h.createDomain))))
	mux.Handle("GET /api/domains/{id}", readLimit(authMiddleware(http.HandlerFunc(h.getDomain))))
	mux.Handle("PUT /api/domains/{id}", rateLimit(authMiddleware(http.HandlerFunc(h.updateDomain))))
	mux.Handle("DELETE /api/domains/{id}", rateLimit(authMiddleware(http.HandlerFunc(h.deleteDomain))))
	mux.Handle("POST /api/domains/{id}/scan", rateLimit(authMiddleware(http.HandlerFunc(h.scanDomain))))
	mux.Handle("GET /api/scanqueue", readLimit(authMiddleware(http.HandlerFunc(h.scanQueueStatus))))

	mux.Handle("GET /api/certificates", readLimit(authMiddleware(http.HandlerFunc(h.listCertificates))))
	mux.Handle("DELETE /api/certificates/errors", rateLimit(authMiddleware(http.HandlerFunc(h.purgeCertificateErrors))))
	mux.Handle("DELETE /api/certificates/expired", rateLimit(authMiddleware(http.HandlerFunc(h.purgeExpiredCertificates))))
	mux.Handle("GET /api/domains/{id}/certificates", readLimit(authMiddleware(http.HandlerFunc(h.listDomainCertificates))))
	mux.Handle("DELETE /api/domains/{id}/certificates/errors", rateLimit(authMiddleware(http.HandlerFunc(h.purgeDomainCertificateErrors))))
	mux.Handle("DELETE /api/domains/{id}/certificates/expired", rateLimit(authMiddleware(http.HandlerFunc(h.purgeDomainExpiredCertificates))))

	mux.Handle("GET /api/dashboard", readLimit(authMiddleware(http.HandlerFunc(h.dashboard))))
	mux.Handle("GET /api/reports/inventory", readLimit(authMiddleware(http.HandlerFunc(h.inventoryReport))))

	h.RegisterUIRoutes(mux)
	h.RegisterDocsRoutes(mux)
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": Version})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if h.db != nil {
		if err := h.db.Ping(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
