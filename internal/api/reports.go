package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/araujofrancisco/certwatch/internal/models"
)

type inventoryEntry struct {
	Domain       *models.Domain     `json:"domain"`
	LatestCert   *models.Certificate `json:"latest_cert"`
	DaysRemaining int                `json:"days_remaining"`
}

type inventoryReport struct {
	Inventory []inventoryEntry `json:"inventory"`
	Summary   summaryStats     `json:"summary"`
}

type summaryStats struct {
	TotalDomains int `json:"total_domains"`
	WithCerts    int `json:"with_certs"`
	Valid        int `json:"valid"`
	Expiring     int `json:"expiring"`
	Expired      int `json:"expired"`
	Errors       int `json:"errors"`
}

func (h *Handler) inventoryReport(w http.ResponseWriter, r *http.Request) {
	domains, err := h.domains.ListDomains()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}

	allCerts, err := h.certs.ListCertificates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list certificates")
		return
	}

	certsByDomain := make(map[int64]*models.Certificate)
	for _, c := range allCerts {
		if existing, ok := certsByDomain[c.DomainID]; ok {
			if c.LastChecked.After(existing.LastChecked) {
				certsByDomain[c.DomainID] = c
			}
		} else {
			certsByDomain[c.DomainID] = c
		}
	}

	now := time.Now()
	entries := make([]inventoryEntry, 0, len(domains))
	var stats summaryStats
	stats.TotalDomains = len(domains)

	for _, d := range domains {
		entry := inventoryEntry{Domain: d, DaysRemaining: 999}
		if c, ok := certsByDomain[d.ID]; ok {
			entry.LatestCert = c
			stats.WithCerts++

			switch {
			case c.Status == "error":
				stats.Errors++
			case c.NotAfter.IsZero():
				stats.Valid++
			case c.NotAfter.Before(now):
				stats.Expired++
			case c.NotAfter.Before(now.AddDate(0, 0, 14)):
				stats.Expiring++
			default:
				stats.Valid++
			}

			if !c.NotAfter.IsZero() {
				if c.NotAfter.Before(now) {
					entry.DaysRemaining = 0
				} else {
					entry.DaysRemaining = int(c.NotAfter.Sub(now).Hours()/24)
				}
			}
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DaysRemaining < entries[j].DaysRemaining
	})

	writeJSON(w, http.StatusOK, inventoryReport{
		Inventory: entries,
		Summary:   stats,
	})
}
