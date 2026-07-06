package notifier

import (
	"time"

	"github.com/araujofrancisco/certwatch/internal/config"
	"github.com/araujofrancisco/certwatch/internal/models"
	"github.com/araujofrancisco/certwatch/internal/templates"
)

type Matcher struct {
	profiles []config.ProfileConfig
}

func NewMatcher(profiles []config.ProfileConfig) *Matcher {
	return &Matcher{profiles: profiles}
}

type MatchResult struct {
	Profile      config.ProfileConfig
	Threshold    int
	Certificates []models.Certificate
}

func (m *Matcher) FindMatches(certs []models.Certificate) []MatchResult {
	var results []MatchResult
	now := time.Now()

	for _, profile := range m.profiles {
		if !profile.Enabled {
			continue
		}
		if profile.Type != "immediate" {
			continue
		}
		for _, t := range profile.Thresholds {
			matched := m.matchThreshold(certs, t, now)
			if len(matched) > 0 {
				results = append(results, MatchResult{
					Profile:      profile,
					Threshold:    t,
					Certificates: matched,
				})
			}
		}
	}
	return results
}

func (m *Matcher) matchThreshold(certs []models.Certificate, threshold int, now time.Time) []models.Certificate {
	var matched []models.Certificate
	deadline := now.AddDate(0, 0, threshold)
	for _, c := range certs {
		if c.NotAfter.IsZero() {
			continue
		}
		if c.NotAfter.After(now) && c.NotAfter.Before(deadline) {
			matched = append(matched, c)
		}
	}
	return matched
}

func (m *Matcher) BuildDailyDigest(certs []models.Certificate, domains []models.Domain) templates.DailySection {
	var section templates.DailySection
	domainMap := make(map[int64]string)
	for _, d := range domains {
		domainMap[d.ID] = d.Domain
	}

	now := time.Now()
	for _, c := range certs {
		if c.NotAfter.IsZero() {
			continue
		}
		days := int(c.NotAfter.Sub(now).Hours() / 24)
		info := templates.CertInfo{
			Domain:      domainMap[c.DomainID],
			Issuer:      c.Issuer,
			Expires:     c.NotAfter,
			DaysRemains: days,
		}
		if days <= 0 {
			section.Critical = append(section.Critical, info)
		} else if days <= 14 {
			section.Warnings = append(section.Warnings, info)
		} else {
			section.Healthy = append(section.Healthy, info)
		}
	}
	return section
}

func (m *Matcher) BuildWeeklyReport(certs []models.Certificate, domains []models.Domain) templates.WeeklyReport {
	var report templates.WeeklyReport
	report.TotalDomains = len(domains)
	now := time.Now()

	for _, c := range certs {
		if c.NotAfter.IsZero() {
			continue
		}
		days := int(c.NotAfter.Sub(now).Hours() / 24)
		if days <= 0 {
			report.Expired++
		} else if days <= 14 {
			report.Warning++
		} else {
			report.Healthy++
		}
	}
	return report
}
