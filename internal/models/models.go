package models

import (
	"time"
)

// User is a local account used for web GUI and API authentication.
// Table name matches the legacy "users" table.
// Name is the unique login username; FullName is the display / real name.
type User struct {
	ID           int64     `gorm:"column:_id;primaryKey;autoIncrement" json:"id"`
	Name         string    `gorm:"column:name;type:text;uniqueIndex;not null" json:"name"`
	FullName     string    `gorm:"column:full_name;type:text" json:"full_name"`
	PasswordHash string    `gorm:"column:password;type:text;not null" json:"-"`
	Active       bool      `gorm:"column:active;not null;default:true" json:"active"`
	IsAdmin      bool      `gorm:"column:is_admin;not null;default:false" json:"is_admin"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string { return "users" }

// Customer is the top-level billing / work entity (client, company, etc.).
type Customer struct {
	ID          int64     `gorm:"column:_id;primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"column:name;type:text;not null" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description"`
	Active      bool      `gorm:"column:active;not null;default:true" json:"active"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Customer) TableName() string { return "customer" }

// Project belongs to a customer (may also represent a cost center).
// Unit is free-text billing unit (e.g. "hours", "kilometers") shown on
// timesheet summaries; the server does not interpret or convert units.
type Project struct {
	ID         int64     `gorm:"column:_id;primaryKey;autoIncrement" json:"id"`
	CustomerID int64     `gorm:"column:customer_id;not null;index" json:"customer_id"`
	Name       string    `gorm:"column:name;type:text;not null" json:"name"`
	CostCenter string    `gorm:"column:costcenter;type:text" json:"cost_center"`
	Unit       string    `gorm:"column:unit;type:text;not null;default:hours" json:"unit"`
	Active     bool      `gorm:"column:active;not null;default:true" json:"active"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Project) TableName() string { return "project" }

// Report is a time entry for a user on a project.
// Soft-delete via Deleted flag (client sync protocol).
// Seq is a monotonic server sequence used by clients for incremental sync.
// ClientUUID is an optional client-generated idempotency key (unique when set).
// Invoiced is set server-side when a timesheet is marked billed (not synced from clients).
type Report struct {
	ID        int64      `gorm:"column:_id;primaryKey;autoIncrement" json:"id"`
	UserID    int64      `gorm:"column:user_id;not null;index" json:"user_id"`
	ProjectID int64      `gorm:"column:project_id;not null;index" json:"project_id"`
	Start     time.Time  `gorm:"column:start;not null;index" json:"start"`
	Stop      *time.Time `gorm:"column:stop" json:"stop"`
	Comment   string     `gorm:"column:comment;type:text" json:"comment"`
	Modified  time.Time  `gorm:"column:modified;not null;index" json:"modified"`
	Seq       int64      `gorm:"column:seq;not null;uniqueIndex" json:"seq"`
	Deleted   bool       `gorm:"column:deleted;not null;default:false" json:"deleted"`
	Invoiced  bool       `gorm:"column:invoiced;not null;default:false" json:"invoiced"`
	// ClientUUID is set by desktop clients for idempotent create (retries after timeouts).
	// Stored as nullable text; uniqueness enforced by a partial index in migrate().
	ClientUUID *string `gorm:"column:client_uuid;type:text" json:"client_uuid,omitempty"`
}

func (Report) TableName() string { return "report" }

// DurationSeconds returns the report length in seconds (0 if still open).
func (r *Report) DurationSeconds() int64 {
	if r.Stop == nil {
		return 0
	}
	return int64(r.Stop.Sub(r.Start).Seconds())
}
