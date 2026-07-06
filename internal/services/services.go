package services

import (
	"github.com/araujofrancisco/certwatch/internal/auth"
	"github.com/araujofrancisco/certwatch/internal/discovery"
	"github.com/araujofrancisco/certwatch/internal/repository"
)

type DomainService struct {
	domains  repository.DomainRepository
	certs    repository.CertificateRepository
	scanners *discovery.Registry
}

type CertificateService struct {
	certs   repository.CertificateRepository
	domains repository.DomainRepository
}

type AuthService struct {
	users repository.UserRepository
	auth  *auth.Authenticator
}

func NewDomainService(domains repository.DomainRepository, certs repository.CertificateRepository, scanners *discovery.Registry) *DomainService {
	return &DomainService{domains: domains, certs: certs, scanners: scanners}
}

func NewCertificateService(certs repository.CertificateRepository, domains repository.DomainRepository) *CertificateService {
	return &CertificateService{certs: certs, domains: domains}
}

func NewAuthService(users repository.UserRepository, a *auth.Authenticator) *AuthService {
	return &AuthService{users: users, auth: a}
}
