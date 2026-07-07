package api

import (
	"net/http"
	"sort"
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
	Healthy      int                  `json:"healthy"`
	Warning      int                  `json:"warning"`
	Expired      int                  `json:"expired"`
	TotalDomains int                  `json:"total_domains"`
	ExpiringSoon []dashboardExpiring  `json:"expiring_soon"`
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	certs, err := h.certs.ListCertificates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list certificates")
		return
	}

	domains, err := h.domains.ListDomains()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}

	domainMap := make(map[int64]string, len(domains))
	for _, d := range domains {
		domainMap[d.ID] = d.Domain
	}

	now := time.Now()
	var healthy, warning, expired int
	var expiring []dashboardExpiring

	for _, c := range certs {
		if c.NotAfter.IsZero() {
			healthy++
			continue
		}
		days := int(c.NotAfter.Sub(now).Hours() / 24)
		switch {
		case days <= 0:
			expired++
		case days <= 14:
			warning++
			expiring = append(expiring, dashboardExpiring{
				DomainID:      c.DomainID,
				Domain:        domainMap[c.DomainID],
				Issuer:        c.Issuer,
				ExpiresAt:     c.NotAfter.Format(time.RFC3339),
				DaysRemaining: days,
			})
		default:
			healthy++
		}
	}

	sort.Slice(expiring, func(i, j int) bool {
		return expiring[i].DaysRemaining < expiring[j].DaysRemaining
	})
	if len(expiring) > 10 {
		expiring = expiring[:10]
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, dashboardResponse{
		Healthy:      healthy,
		Warning:      warning,
		Expired:      expired,
		TotalDomains: len(domains),
		ExpiringSoon: expiring,
	})
}
