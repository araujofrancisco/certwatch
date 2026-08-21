package services

import (
	"context"
	"time"

	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/repository"
)

type DomainService struct {
	domains       repository.DomainRepository
	certs         repository.CertificateRepository
	scanners      *discovery.Registry
	tags          repository.TagRepository
	backgroundCtx context.Context
	scanQueue     *scanQueue
}

type CertificateService struct {
	certs   repository.CertificateRepository
	domains repository.DomainRepository
}

type AuthService struct {
	users repository.UserRepository
	auth  *auth.Authenticator
}

func NewDomainService(domains repository.DomainRepository, certs repository.CertificateRepository, scanners *discovery.Registry, tags repository.TagRepository, backgroundCtx context.Context, maxConcurrent, queueSize int, scanTimeout time.Duration) *DomainService {
	// backgroundCtx should be a long-lived context (e.g. context.Background()
	// or the process lifetime context), NOT a per-request or signal context:
	// scans queued with it must survive request completion and run to
	// completion during StopScanQueue at shutdown.
	s := &DomainService{domains: domains, certs: certs, scanners: scanners, tags: tags, backgroundCtx: backgroundCtx}
	s.scanQueue = newScanQueue(maxConcurrent, queueSize, scanTimeout, s.ScanDomain)
	return s
}

// EnqueueScan queues a background scan for the given domain. priority marks a
// manual "Scan Now" request that bypasses the low-priority (bulk/periodic) queue.
func (s *DomainService) EnqueueScan(ctx context.Context, domainID int64, priority bool) {
	s.scanQueue.EnqueueScan(ctx, domainID, priority)
}

// EnqueueScanBackground queues a scan tied to the service's background context
// rather than a request context. Request contexts are cancelled as soon as the
// HTTP handler returns, which would abort a queued scan before it runs.
func (s *DomainService) EnqueueScanBackground(domainID int64, priority bool) {
	s.scanQueue.EnqueueScan(s.backgroundCtx, domainID, priority)
}

// StopScanQueue shuts down the background scan queue, blocking until in-flight
// scans complete (honoring their contexts).
func (s *DomainService) StopScanQueue() {
	if s.scanQueue != nil {
		s.scanQueue.Stop()
	}
}

// ScanQueueStats reports the current size of the pending and in-flight scan queues.
func (s *DomainService) ScanQueueStats() map[string]int {
	if s.scanQueue == nil {
		return map[string]int{"pending": 0, "inflight": 0}
	}
	return map[string]int{"pending": s.scanQueue.Pending(), "inflight": s.scanQueue.InFlight()}
}

func NewCertificateService(certs repository.CertificateRepository, domains repository.DomainRepository) *CertificateService {
	return &CertificateService{certs: certs, domains: domains}
}

func NewAuthService(users repository.UserRepository, a *auth.Authenticator) *AuthService {
	return &AuthService{users: users, auth: a}
}
