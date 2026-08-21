package api

import (
	"net/http"
	"time"
)

type dashboardExpiring struct {
	DomainID      int64  `json:"domain_id"`
	Domain        string `json:"domain"`
	Issuer        string `json:"issuer"`
	ExpiresAt     string `json:"expires_at"`
	DaysRemaining int    `json:"days_remaining"`
}

type dashboardResponse struct {
	Healthy      int                 `json:"healthy"`
	Warning      int                 `json:"warning"`
	Expired      int                 `json:"expired"`
	TotalDomains int                 `json:"total_domains"`
	ExpiringSoon []dashboardExpiring `json:"expiring_soon"`
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	// Bucket boundaries mirror the original in-memory logic: days remaining
	// is computed as int(hours/24), so "expired" covers < 24h of life left
	// and "warning" covers 24h up to (but excluding) 360h.
	now := time.Now()
	warningStart := now.Add(24 * time.Hour)
	warningEnd := now.Add(360 * time.Hour)

	counts, err := h.certs.CountExpiryBuckets(r.Context(), warningStart, warningEnd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute dashboard counts")
		return
	}

	totalDomains, err := h.domains.CountDomains(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count domains")
		return
	}

	rows, err := h.certs.ListExpiringSoon(r.Context(), now, warningEnd, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list expiring certificates")
		return
	}

	expiring := make([]dashboardExpiring, 0, len(rows))
	for _, e := range rows {
		expiring = append(expiring, dashboardExpiring{
			DomainID:      e.DomainID,
			Domain:        e.Domain,
			Issuer:        e.Issuer,
			ExpiresAt:     e.ExpiresAt.Format(time.RFC3339),
			DaysRemaining: int(e.ExpiresAt.Sub(now).Hours() / 24),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, dashboardResponse{
		Healthy:      counts.Healthy,
		Warning:      counts.Warning,
		Expired:      counts.Expired,
		TotalDomains: totalDomains,
		ExpiringSoon: expiring,
	})
}
