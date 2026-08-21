package repository

import (
	"context"
	"time"

	"github.com/araujofrancisco/certwatch/internal/database"
	"github.com/araujofrancisco/certwatch/internal/models"
)

type UserRepository interface {
	Create(ctx context.Context, u *models.User) error
	FindByID(ctx context.Context, id int64) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
}

type DomainRepository interface {
	Create(ctx context.Context, d *models.Domain) error
	CreateMany(ctx context.Context, domains []*models.Domain) error
	FindByID(ctx context.Context, id int64) (*models.Domain, error)
	FindByDomain(ctx context.Context, domain string) (*models.Domain, error)
	List(ctx context.Context) ([]*models.Domain, error)
	ListEnabled(ctx context.Context) ([]*models.Domain, error)
	ListFiltered(ctx context.Context, filter models.DomainFilter) ([]*models.Domain, error)
	CountFiltered(ctx context.Context, filter models.DomainFilter) (int, error)
	Update(ctx context.Context, d *models.Domain) error
	Delete(ctx context.Context, id int64) error
}

type CertificateRepository interface {
	Create(ctx context.Context, c *models.Certificate) error
	FindByID(ctx context.Context, id int64) (*models.Certificate, error)
	ListByDomainID(ctx context.Context, domainID int64) ([]*models.Certificate, error)
	List(ctx context.Context) ([]*models.Certificate, error)
	ListLatestByDomain(ctx context.Context) ([]*models.Certificate, error)
	ListFiltered(ctx context.Context, filter models.CertFilter) ([]*models.Certificate, error)
	CountFiltered(ctx context.Context, filter models.CertFilter) (int, error)
	CountExpiryBuckets(ctx context.Context, warningStart, warningEnd time.Time) (CertBucketCounts, error)
	ListExpiringSoon(ctx context.Context, from, before time.Time, limit int) ([]ExpiringSoonCert, error)
	Update(ctx context.Context, c *models.Certificate) error
	Delete(ctx context.Context, id int64) error
	DeleteErrors(ctx context.Context) (int64, error)
	DeleteErrorsByDomain(ctx context.Context, domainID int64) (int64, error)
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteExpiredByDomain(ctx context.Context, domainID int64) (int64, error)
}

type TagRepository interface {
	Create(ctx context.Context, name, color string) (*models.Tag, error)
	FindByID(ctx context.Context, id int64) (*models.Tag, error)
	FindByName(ctx context.Context, name string) (*models.Tag, error)
	List(ctx context.Context) ([]*models.Tag, error)
	Delete(ctx context.Context, id int64) error
	SetDomainTags(ctx context.Context, domainID int64, tagIDs []int64) error
	GetDomainTags(ctx context.Context, domainID int64) ([]*models.Tag, error)
	GetTagsByDomainIDs(ctx context.Context, domainIDs []int64) (map[int64][]*models.Tag, error)
}

type NotificationDedupRepository interface {
	Seen(ctx context.Context, key string) (bool, error)
	Mark(ctx context.Context, key string, at time.Time) error
	Cleanup(ctx context.Context, before time.Time) (int64, error)
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

func NewTagRepository(db *database.DB) TagRepository {
	return &tagRepo{db: db}
}
