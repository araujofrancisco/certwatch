package models

import "time"

type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Domain struct {
	ID          int64     `json:"id"`
	Domain      string    `json:"domain"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Certificate struct {
	ID          int64     `json:"id"`
	DomainID    int64     `json:"domain_id"`
	Issuer      string    `json:"issuer"`
	Subject     string    `json:"subject"`
	Serial      string    `json:"serial"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	Fingerprint string    `json:"fingerprint"`
	Protocol    string    `json:"protocol"`
	Status      string    `json:"status"`
	LastChecked time.Time `json:"last_checked"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DomainFilter struct {
	Query   string // search domain + description
	Enabled *bool  // nil = all, non-nil = filter
}

type CertFilter struct {
	Query    string // search subject + issuer
	DomainID *int64 // nil = all domains
	Status   string // filter by status
	Protocol string // filter by protocol
	Expiring int    // >0: expiring within N days
	Expired  bool   // only expired certs
}

type NotificationProfile struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Enabled    bool      `json:"enabled"`
	Recipients string    `json:"recipients"`
	Config     string    `json:"config"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
