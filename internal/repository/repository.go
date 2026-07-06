package repository

import (
	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type UserRepository interface {
	Create(u *models.User) error
	FindByID(id int64) (*models.User, error)
	FindByEmail(email string) (*models.User, error)
	List() ([]*models.User, error)
	Update(u *models.User) error
	Delete(id int64) error
}

type DomainRepository interface {
	Create(d *models.Domain) error
	FindByID(id int64) (*models.Domain, error)
	FindByDomain(domain string) (*models.Domain, error)
	List() ([]*models.Domain, error)
	ListEnabled() ([]*models.Domain, error)
	ListFiltered(filter models.DomainFilter) ([]*models.Domain, error)
	Update(d *models.Domain) error
	Delete(id int64) error
}

type CertificateRepository interface {
	Create(c *models.Certificate) error
	FindByID(id int64) (*models.Certificate, error)
	ListByDomainID(domainID int64) ([]*models.Certificate, error)
	LatestForDomain(domainID int64) (*models.Certificate, error)
	List() ([]*models.Certificate, error)
	ListFiltered(filter models.CertFilter) ([]*models.Certificate, error)
	Update(c *models.Certificate) error
	Delete(id int64) error
	DeleteErrors() (int64, error)
	DeleteErrorsByDomain(domainID int64) (int64, error)
}

type NotificationProfileRepository interface {
	Create(p *models.NotificationProfile) error
	FindByID(id int64) (*models.NotificationProfile, error)
	List() ([]*models.NotificationProfile, error)
	Update(p *models.NotificationProfile) error
	Delete(id int64) error
}

func NewUserRepository(db *database.DB) UserRepository {
	return &userRepo{db: db}
}

func NewDomainRepository(db *database.DB) DomainRepository {
	return &domainRepo{db: db}
}

func NewCertificateRepository(db *database.DB) CertificateRepository {
	return &certRepo{db: db}
}

func NewNotificationProfileRepository(db *database.DB) NotificationProfileRepository {
	return &notifProfileRepo{db: db}
}
