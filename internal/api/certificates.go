package api

import (
	"net/http"
	"strconv"

	"github.com/araujofrancisco/certwatch/internal/models"
)

func (h *Handler) listCertificates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var domainID *int64
	if v := q.Get("domain_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			domainID = &id
		}
	}

	f := models.CertFilter{
		Query:    q.Get("q"),
		DomainID: domainID,
		Status:   q.Get("status"),
		Protocol: q.Get("protocol"),
	}

	if v := q.Get("expiring"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Expiring = n
		}
	}
	if q.Get("expired") == "true" || q.Get("expired") == "1" {
		f.Expired = true
	}

	certs, err := h.certs.ListCertificatesFiltered(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list certificates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"certificates": certs})
}

func (h *Handler) listDomainCertificates(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid domain id")
		return
	}

	q := r.URL.Query()

	var domainID *int64
	if v := q.Get("domain_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			domainID = &n
		}
	}
	if domainID == nil {
		domainID = &id
	}

	f := models.CertFilter{
		Query:    q.Get("q"),
		DomainID: domainID,
		Status:   q.Get("status"),
		Protocol: q.Get("protocol"),
	}

	if v := q.Get("expiring"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Expiring = n
		}
	}
	if q.Get("expired") == "true" || q.Get("expired") == "1" {
		f.Expired = true
	}

	certs, err := h.certs.ListCertificatesFiltered(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list certificates")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"certificates": certs})
}

func (h *Handler) purgeCertificateErrors(w http.ResponseWriter, r *http.Request) {
	n, err := h.certs.PurgeErrors()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to purge certificate errors")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

func (h *Handler) purgeDomainCertificateErrors(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid domain id")
		return
	}

	n, err := h.certs.PurgeErrorsByDomain(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to purge certificate errors")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}
