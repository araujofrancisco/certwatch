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

	// Embed the IANA timezone database: the scratch container image ships no
	// /usr/share/zoneinfo, so time.LoadLocation would fail for every
	// notification profile timezone without this.
	_ "time/tzdata"

	"github.com/araujofrancisco/certwatch/internal/api"
	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/config"
	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/logging"
	"github.com/araujofrancisco/certwatch/internal/middleware"
	"github.com/araujofrancisco/certwatch/internal/notifier"
	"github.com/araujofrancisco/certwatch/internal/repository"
	"github.com/araujofrancisco/certwatch/internal/services"
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
			cfgPath = config.DefaultPath
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
	cfgPath := config.DefaultPath
	if v := os.Getenv("CERTWATCH_CONFIG"); v != "" {
		cfgPath = v
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	logging.Init(cfg.Logging.Level, cfg.Logging.Format)

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	for _, w := range cfg.Warnings() {
		slog.Warn(w)
	}

	slog.Info("starting certwatch", "version", version())
	slog.Info("configuration", "config", cfg)

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
	scannerReg.Register(discovery.NewCTScanner(scanTimeout))

	userRepo := repository.NewUserRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	certRepo := repository.NewCertificateRepository(db)
	tagRepo := repository.NewTagRepository(db)

	authSvc := services.NewAuthService(userRepo, authenticator)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// The domain service gets its own background context rather than the
	// signal context: queued and in-flight scans must survive SIGTERM so
	// StopScanQueue can drain them cleanly during shutdown.
	domainSvc := services.NewDomainService(domainRepo, certRepo, scannerReg, tagRepo, context.Background(), cfg.Discovery.MaxConcurrentScans, cfg.Discovery.QueueSize, scanTimeout)
	certSvc := services.NewCertificateService(certRepo, domainRepo)

	rateLimit := cfg.Server.RateLimit
	if rateLimit <= 0 {
		rateLimit = 10
	}
	readRateLimit := cfg.Server.ReadRateLimit
	if readRateLimit <= 0 {
		readRateLimit = 300
	}
	rateWindow := time.Minute
	if w, err := time.ParseDuration(cfg.Server.RateLimitWindow); err == nil && w > 0 {
		rateWindow = w
	}
	rateLimiter := middleware.NewRateLimiter(rateLimit, rateWindow)
	readRateLimiter := middleware.NewRateLimiter(readRateLimit, rateWindow) // Higher limit for GET requests
	handler := api.NewHandler(domainSvc, certSvc, authSvc, authenticator, db.DB, rateLimiter, readRateLimiter, cfg.Server.TrustProxyHeaders)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	corsOrigins := cfg.Server.CORSAllowedOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:8080", "http://127.0.0.1:8080"}
	}
	requestTimeout := 30 * time.Second
	if t, err := time.ParseDuration(cfg.Server.RequestTimeout); err == nil && t > 0 {
		requestTimeout = t
	}
	wrapped := middleware.Recovery(middleware.Logging(middleware.SecurityHeaders(middleware.Timeout(requestTimeout)(middleware.CORS(corsOrigins, cfg.Server.AllowLocalhostOrigins)(mux)))))

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

	go func() {
		slog.Info("listening", "addr", cfg.ServerAddr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
		}
	}()

	go runBackgroundScan(ctx, domainSvc, certSvc, scanTimeout, scanInterval)
	dedupRepo := repository.NewNotificationDedupRepository(db)
	go notifier.NewRunner(cfg.Notifications, certSvc, domainSvc, dedupRepo).Run(ctx)

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	domainSvc.StopScanQueue()
	rateLimiter.Stop()
	return nil
}

func runBackgroundScan(ctx context.Context, svc *services.DomainService, certSvc *services.CertificateService, timeout, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			slog.Info("starting background scan")
			if err := svc.ScanAllDomains(ctx); err != nil {
				slog.Error("background scan error", "error", err)
				continue
			}
			slog.Info("background scan queued", "queued", svc.ScanQueueStats()["pending"])

			// Safety net: purge all expired certificates that were not
			// caught by the renewal-triggered cleanup (e.g. domains whose
			// cert expired between scan cycles).
			if n, err := certSvc.PurgeExpired(ctx); err != nil {
				slog.Error("failed to purge expired certificates", "error", err)
			} else if n > 0 {
				slog.Info("purged expired certificates", "deleted", n)
			}
		}
	}
}

func version() string {
	return api.Version
}
