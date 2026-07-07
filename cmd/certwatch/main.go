package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/araujofrancisco/certwatch/internal/api"
	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/config"
	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/logging"
	"github.com/araujofrancisco/certwatch/internal/middleware"
	"github.com/araujofrancisco/certwatch/internal/models"
	"github.com/araujofrancisco/certwatch/internal/notifier"
	"github.com/araujofrancisco/certwatch/internal/repository"
	"github.com/araujofrancisco/certwatch/internal/scheduler"
	"github.com/araujofrancisco/certwatch/internal/services"
	"github.com/araujofrancisco/certwatch/internal/templates"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		os.Exit(healthCheck())
	}
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func healthCheck() int {
	port := os.Getenv("CERTWATCH_SERVER_PORT")
	if port == "" {
		cfgPath := os.Getenv("CERTWATCH_CONFIG")
		if cfgPath == "" {
			cfgPath = "config/default.yaml"
		}
		cfg, err := config.Load(cfgPath)
		if err == nil {
			port = fmt.Sprintf("%d", cfg.Server.Port)
		}
	}
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:" + port + "/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "unexpected status: %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

func run() error {
	cfgPath := "config/default.yaml"
	if v := os.Getenv("CERTWATCH_CONFIG"); v != "" {
		cfgPath = v
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	logging.Init(cfg.Logging.Level, cfg.Logging.Format)

	if cfg.Auth.Secret == "change-me-in-production" {
		slog.Warn("using default JWT secret — set CERTWATCH_AUTH_SECRET in production")
	}

	slog.Info("starting certwatch", "version", version())

	if err := database.EnsureDir(cfg.Database.Driver, cfg.Database.DSN); err != nil {
		return err
	}

	db, err := database.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		return err
	}

	tokenTTL, err := time.ParseDuration(cfg.Auth.TokenTTL)
	if err != nil {
		tokenTTL = 24 * time.Hour
	}
	authenticator := auth.New(cfg.Auth.Secret, tokenTTL)

	scanTimeout, err := time.ParseDuration(cfg.Discovery.Timeout)
	if err != nil {
		scanTimeout = 30 * time.Second
	}

	scannerReg := discovery.NewRegistry()
	scannerReg.Register(discovery.NewHTTPSScanner(scanTimeout))
	scannerReg.Register(discovery.NewSMTPScanner(scanTimeout))
	scannerReg.Register(discovery.NewIMAPScanner(scanTimeout))
	scannerReg.Register(discovery.NewPOP3Scanner(scanTimeout))
	scannerReg.Register(discovery.NewLDAPScanner(scanTimeout))
	scannerReg.Register(discovery.NewFTPScanner(scanTimeout))
	scannerReg.Register(discovery.NewTLSScanner(scanTimeout))
	scannerReg.Register(discovery.NewCTScanner(scanTimeout))

	userRepo := repository.NewUserRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	authSvc := services.NewAuthService(userRepo, authenticator)
	domainSvc := services.NewDomainService(domainRepo, certRepo, scannerReg, tagRepo)
	certSvc := services.NewCertificateService(certRepo, domainRepo)

	rateLimiter := middleware.NewRateLimiter(10, time.Minute)
	handler := api.NewHandler(domainSvc, certSvc, authSvc, authenticator, db.DB, rateLimiter)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	corsOrigins := cfg.Server.CORSAllowedOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:8080", "http://127.0.0.1:8080"}
	}
	wrapped := middleware.Recovery(middleware.Logging(middleware.SecurityHeaders(middleware.CORS(corsOrigins)(mux))))

	srv := &http.Server{
		Addr:         cfg.ServerAddr(),
		Handler:      wrapped,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	scanInterval, err := time.ParseDuration(cfg.Discovery.ScanInterval)
	if err != nil {
		scanInterval = 6 * time.Hour
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", cfg.ServerAddr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
		}
	}()

	go runBackgroundScan(ctx, domainSvc, scanTimeout, scanInterval)
	go runNotifications(ctx, cfg, domainSvc, certSvc)

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

func runBackgroundScan(ctx context.Context, svc *services.DomainService, timeout, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			slog.Info("starting background scan")
			certs, err := svc.ScanAllDomains(ctx, timeout)
			if err != nil {
				slog.Error("background scan error", "error", err)
				continue
			}
			slog.Info("background scan complete", "certificates_found", len(certs))
		}
	}
}

func runNotifications(ctx context.Context, cfg config.Config, domainSvc *services.DomainService, certSvc *services.CertificateService) {
	notified := make(map[string]bool) // key: "certID:threshold" — dedup across minutes

	if err := notifier.ValidateProfiles(cfg.Notifications.Profiles); err != nil {
		slog.Error("invalid notification profiles", "error", err)
		return
	}

	notifierN := notifier.New(cfg.Notifications)
	matcher := notifier.NewMatcher(notifier.FilterEnabled(cfg.Notifications.Profiles))

	sched := scheduler.New()

	for _, p := range notifier.FilterEnabled(cfg.Notifications.Profiles) {
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
					checkImmediateNotifications(ctx, domainSvc, certSvc, notifierN, matcher, profile, notified)
				},
			})
			continue
		}

		cronExpr := notifier.DefaultCron(profile)
		expr, err := scheduler.ParseCron(cronExpr)
		if err != nil {
			slog.Error("parse cron for profile", "name", profile.Name, "error", err)
			continue
		}
		loc, _ := time.LoadLocation("America/New_York")
		if loc == nil {
			loc = time.FixedZone("America/New_York", -5*60*60)
		}
		sched.Add(&scheduler.Job{
			Name:     profile.Name,
			Expr:     expr,
			Timezone: loc,
			Handler: func(ctx context.Context) {
				sendDigest(ctx, domainSvc, certSvc, notifierN, matcher, profile)
			},
		})
	}

	slog.Info("starting notification scheduler")
	sched.Start(ctx)
}

func checkImmediateNotifications(ctx context.Context, domainSvc *services.DomainService, certSvc *services.CertificateService, notifierN *notifier.Notifier, matcher *notifier.Matcher, profile config.ProfileConfig, notified map[string]bool) {
	certs, err := certSvc.ListCertificates()
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

	for _, m := range matches {
		if m.Profile.Name != profile.Name {
			continue
		}
		for _, c := range m.Certificates {
			key := fmt.Sprintf("%d:%d", c.ID, m.Threshold)
			if notified[key] {
				continue
			}
			notified[key] = true

			domain, err := domainSvc.GetDomain(c.DomainID)
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
			if err := notifierN.SendEmail(ctx, m.Profile.Recipients, subject, body); err != nil {
				slog.Error("send immediate notification", "error", err)
			}
		}
	}
}

func sendDigest(ctx context.Context, domainSvc *services.DomainService, certSvc *services.CertificateService, notifierN *notifier.Notifier, matcher *notifier.Matcher, profile config.ProfileConfig) {
	certs, err := certSvc.ListCertificates()
	if err != nil {
		slog.Error("digest: list certificates", "error", err)
		return
	}
	domains, err := domainSvc.ListDomains()
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
		if err := notifierN.SendEmail(ctx, profile.Recipients, subject, body); err != nil {
			slog.Error("send daily digest", "error", err)
		}
	case "weekly-digest":
		report := matcher.BuildWeeklyReport(allCerts, allDomains)
		subject, body := templates.WeeklyReportDigest(report)
		if err := notifierN.SendEmail(ctx, profile.Recipients, subject, body); err != nil {
			slog.Error("send weekly digest", "error", err)
		}
	}
}

func version() string {
	return "0.1.0"
}
