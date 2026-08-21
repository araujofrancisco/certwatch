package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/araujofrancisco/certwatch/internal/config"
	"github.com/araujofrancisco/certwatch/internal/models"
	"github.com/araujofrancisco/certwatch/internal/scheduler"
	"github.com/araujofrancisco/certwatch/internal/templates"
)

// DedupStore persists notification dedup keys across restarts.
type DedupStore interface {
	Seen(ctx context.Context, key string) (bool, error)
	Mark(ctx context.Context, key string, at time.Time) error
	Cleanup(ctx context.Context, before time.Time) (int64, error)
}

// CertificateLister supplies the certificates to match against.
type CertificateLister interface {
	ListCertificates(ctx context.Context) ([]*models.Certificate, error)
}

// DomainLookup resolves domain records and lists all domains for digests.
type DomainLookup interface {
	GetDomain(ctx context.Context, id int64) (*models.Domain, error)
	ListDomains(ctx context.Context) ([]*models.Domain, error)
}

// dedupTTL is how long a dedup key suppresses repeat notifications.
const dedupTTL = 24 * time.Hour

// Runner owns notification orchestration: profile validation, scheduling and
// dispatch of immediate alerts and digests. The entrypoint only constructs it
// and calls Run.
type Runner struct {
	cfg     config.NotificationsConfig
	certs   CertificateLister
	domains DomainLookup
	dedup   DedupStore
}

func NewRunner(cfg config.NotificationsConfig, certs CertificateLister, domains DomainLookup, dedup DedupStore) *Runner {
	return &Runner{cfg: cfg, certs: certs, domains: domains, dedup: dedup}
}

// Run validates profiles, registers scheduler jobs and blocks until ctx is
// cancelled. Intended to be called from a dedicated goroutine.
func (r *Runner) Run(ctx context.Context) {
	if err := ValidateProfiles(r.cfg.Profiles); err != nil {
		slog.Error("invalid notification profiles", "error", err)
		return
	}

	n := New(r.cfg)
	matcher := NewMatcher(FilterEnabled(r.cfg.Profiles))
	sched := scheduler.New()

	go r.cleanupLoop(ctx)

	for _, p := range FilterEnabled(r.cfg.Profiles) {
		profile := p

		if profile.Type == "immediate" {
			expr, err := scheduler.ParseCron("* * * * *")
			if err != nil {
				slog.Error("parse immediate cron", "error", err)
				continue
			}
			sched.Add(&scheduler.Job{
				Name:     profile.Name + "-immediate",
				Expr:     expr,
				Timezone: time.UTC,
				Handler: func(ctx context.Context) {
					r.checkImmediate(ctx, n, matcher, profile)
				},
			})
			continue
		}

		cronExpr := DefaultCron(profile)
		expr, err := scheduler.ParseCron(cronExpr)
		if err != nil {
			slog.Error("parse cron for profile", "name", profile.Name, "error", err)
			continue
		}
		locName := profile.Timezone
		if locName == "" {
			locName = "America/New_York"
		}
		loc, err := time.LoadLocation(locName)
		if err != nil {
			slog.Warn("invalid timezone, falling back to America/New_York", "profile", profile.Name, "timezone", locName)
			loc = time.FixedZone("America/New_York", -5*60*60)
		}
		sched.Add(&scheduler.Job{
			Name:     profile.Name,
			Expr:     expr,
			Timezone: loc,
			Handler: func(ctx context.Context) {
				r.sendDigest(ctx, n, matcher, profile)
			},
		})
	}

	slog.Info("starting notification scheduler")
	sched.Start(ctx)
}

// cleanupLoop periodically removes expired dedup keys from the store.
func (r *Runner) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.dedup == nil {
				return
			}
			if n, err := r.dedup.Cleanup(ctx, time.Now().Add(-dedupTTL)); err != nil {
				slog.Error("notification dedup cleanup", "error", err)
			} else if n > 0 {
				slog.Debug("notification dedup cleanup", "deleted", n)
			}
		}
	}
}

func (r *Runner) checkImmediate(ctx context.Context, n *Notifier, matcher *Matcher, profile config.ProfileConfig) {
	certs, err := r.certs.ListCertificates(ctx)
	if err != nil {
		slog.Error("immediate check: list certificates", "error", err)
		return
	}

	var allCerts []models.Certificate
	for _, c := range certs {
		allCerts = append(allCerts, *c)
	}

	matches := matcher.FindMatches(allCerts)
	if len(matches) == 0 {
		return
	}

	now := time.Now()
	for _, m := range matches {
		if m.Profile.Name != profile.Name {
			continue
		}
		for _, c := range m.Certificates {
			key := fmt.Sprintf("%d:%d", c.ID, m.Threshold)
			if r.seen(ctx, key) {
				continue
			}
			r.mark(ctx, key, now)

			domain, err := r.domains.GetDomain(ctx, c.DomainID)
			if err != nil {
				continue
			}
			info := templates.CertInfo{
				Domain:      domain.Domain,
				Issuer:      c.Issuer,
				Expires:     c.NotAfter,
				DaysRemains: m.Threshold,
			}
			subject, body := templates.ImmediateAlert(info)
			if err := n.SendEmail(ctx, m.Profile.Recipients, subject, body); err != nil {
				slog.Error("send immediate notification", "error", err)
			}
		}
	}
}

// seen reports whether a key was notified within the TTL window. Store errors
// are logged and treated as "not seen" so delivery is never suppressed by an
// infrastructure failure (at-least-once semantics).
func (r *Runner) seen(ctx context.Context, key string) bool {
	if r.dedup == nil {
		return false
	}
	v, err := r.dedup.Seen(ctx, key)
	if err != nil {
		slog.Error("notification dedup lookup", "key", key, "error", err)
		return false
	}
	return v
}

// mark records a key as notified; failures are logged but do not abort the
// notification that was already accepted for sending.
func (r *Runner) mark(ctx context.Context, key string, at time.Time) {
	if r.dedup == nil {
		return
	}
	if err := r.dedup.Mark(ctx, key, at); err != nil {
		slog.Error("notification dedup mark", "key", key, "error", err)
	}
}

func (r *Runner) sendDigest(ctx context.Context, n *Notifier, matcher *Matcher, profile config.ProfileConfig) {
	certs, err := r.certs.ListCertificates(ctx)
	if err != nil {
		slog.Error("digest: list certificates", "error", err)
		return
	}
	domains, err := r.domains.ListDomains(ctx)
	if err != nil {
		slog.Error("digest: list domains", "error", err)
		return
	}

	var allCerts []models.Certificate
	for _, c := range certs {
		allCerts = append(allCerts, *c)
	}
	var allDomains []models.Domain
	for _, d := range domains {
		allDomains = append(allDomains, *d)
	}

	switch profile.Type {
	case "daily-digest":
		section := matcher.BuildDailyDigest(allCerts, allDomains)
		subject, body := templates.DailyDigest(time.Now(), section)
		if err := n.SendEmail(ctx, profile.Recipients, subject, body); err != nil {
			slog.Error("send daily digest", "error", err)
		}
	case "weekly-digest":
		report := matcher.BuildWeeklyReport(allCerts, allDomains)
		subject, body := templates.WeeklyReportDigest(report)
		if err := n.SendEmail(ctx, profile.Recipients, subject, body); err != nil {
			slog.Error("send weekly digest", "error", err)
		}
	}
}
