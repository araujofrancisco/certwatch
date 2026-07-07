package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/araujofrancisco/certwatch/internal/models"
	"github.com/araujofrancisco/certwatch/internal/services"
)

type createDomainRequest struct {
	Domain      string   `json:"domain"`
	Description string   `json:"description"`
	Group       string   `json:"group"`
	Tags        []string `json:"tags,omitempty"`
}

type updateDomainRequest struct {
	Domain      string   `json:"domain"`
	Description string   `json:"description"`
	Group       string   `json:"group"`
	Enabled     bool     `json:"enabled"`
	Tags        []string `json:"tags,omitempty"`
}

func (h *Handler) listDomains(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var enabled *bool
	if v := q.Get("enabled"); v != "" {
		b := v == "true" || v == "1"
		enabled = &b
	}

	f := models.DomainFilter{
		Query:   q.Get("q"),
		Enabled: enabled,
	}

	if page, err := strconv.Atoi(q.Get("page")); err == nil && page > 0 {
		f.Page = page
	}
	if limit, err := strconv.Atoi(q.Get("limit")); err == nil && limit > 0 {
		f.Limit = limit
	}

	domains, err := h.domains.ListDomainsPaginated(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *Handler) createDomain(w http.ResponseWriter, r *http.Request) {
	var req createDomainRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	domain, err := h.domains.AddDomain(req.Domain, req.Description, req.Group)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Tags) > 0 {
		if err := h.domains.SetDomainTags(domain.ID, req.Tags); err != nil {
			slog.Error("failed to set domain tags", "domain_id", domain.ID, "error", err)
		}
	}

	domain, err = h.domains.GetDomain(domain.ID)
	if err != nil {
		slog.Error("failed to re-fetch domain", "domain_id", domain.ID, "error", err)
	}

	go func(id int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := h.domains.ScanDomain(ctx, id, 30*time.Second); err != nil {
			slog.Error("background scan failed", "domain_id", id, "error", err)
		}
	}(domain.ID)

	writeJSON(w, http.StatusCreated, map[string]any{"domain": domain})
}

func (h *Handler) updateDomain(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid domain id")
		return
	}

	var req updateDomainRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	domain, err := h.domains.UpdateDomain(id, req.Domain, req.Description, req.Group, req.Enabled, req.Tags)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"domain": domain})
}

func (h *Handler) importDomains(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var raw struct {
		Domains json.RawMessage `json:"domains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil || len(raw.Domains) == 0 {
		writeError(w, http.StatusBadRequest, "missing domains")
		return
	}

	var pairs []services.BulkDomainEntry

	var objs []struct {
		Domain      string   `json:"domain"`
		Description string   `json:"description"`
		Group       string   `json:"group"`
		Tags        []string `json:"tags,omitempty"`
	}
	if err := json.Unmarshal(raw.Domains, &objs); err == nil {
		for _, o := range objs {
			pairs = append(pairs, services.BulkDomainEntry{
				Domain:      o.Domain,
				Description: o.Description,
				Group:       o.Group,
				Tags:        o.Tags,
			})
		}
	} else {
		var strs []string
		if err := json.Unmarshal(raw.Domains, &strs); err != nil {
			writeError(w, http.StatusBadRequest, "domains must be an array of objects or strings")
			return
		}
		for _, s := range strs {
			pairs = append(pairs, services.BulkDomainEntry{Domain: s})
		}
	}

	if len(pairs) == 0 {
		writeError(w, http.StatusBadRequest, "missing domains")
		return
	}

	result := h.domains.BulkAddDomains(pairs)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getDomain(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid domain id")
		return
	}

	domain, err := h.domains.GetDomain(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"domain": domain})
}

func (h *Handler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid domain id")
		return
	}

	if err := h.domains.DeleteDomain(id); err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) scanDomain(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid domain id")
		return
	}

	cert, err := h.domains.ScanDomain(r.Context(), id, 30*time.Second)
	if err != nil {
		if cert != nil {
			writeJSON(w, http.StatusBadGateway,
				map[string]any{"certificate": cert, "scan_error": err.Error()})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"certificate": cert})
}
