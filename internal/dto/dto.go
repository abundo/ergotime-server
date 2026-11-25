package dto

import (
	"fmt"
	"strings"
	"time"

	"github.com/abundo/ergotime-server/internal/models"
)

// Envelope is the standard list response (compatible with legacy { "data": [...] }).
type Envelope struct {
	Data any `json:"data"`
}

// ErrorBody is a JSON error payload.
type ErrorBody struct {
	Message string `json:"message"`
}

// LoginRequest is the SPA / API login body.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is returned after successful authentication.
type LoginResponse struct {
	User UserPublic `json:"user"`
}

// UserPublic is a user without password hash.
type UserPublic struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Active   bool   `json:"active"`
	IsAdmin  bool   `json:"is_admin"`
	// Timezone is an IANA name. Empty means "use the browser timezone".
	Timezone string `json:"timezone"`
}

func UserFromModel(u *models.User) UserPublic {
	return UserPublic{
		ID:       u.ID,
		Name:     u.Name,
		FullName: u.FullName,
		Active:   u.Active,
		IsAdmin:  u.IsAdmin,
		Timezone: u.Timezone,
	}
}

// CreateUserRequest creates a new local user.
type CreateUserRequest struct {
	Name     string  `json:"name"`
	FullName string  `json:"full_name"`
	Password string  `json:"password"`
	Active   *bool   `json:"active"`
	IsAdmin  *bool   `json:"is_admin"`
	Timezone *string `json:"timezone"`
}

// UpdateUserRequest updates user fields (password optional).
type UpdateUserRequest struct {
	Name     *string `json:"name"`
	FullName *string `json:"full_name"`
	Password *string `json:"password"`
	Active   *bool   `json:"active"`
	IsAdmin  *bool   `json:"is_admin"`
	// Timezone is an IANA name. An empty string clears the profile and the
	// browser timezone is used for display.
	Timezone *string `json:"timezone"`
}

// CustomerRequest is create/update payload for customers.
type CustomerRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      *bool  `json:"active"`
}

// ProjectRequest is create/update payload for projects.
type ProjectRequest struct {
	CustomerID int64  `json:"customer_id"`
	Name       string `json:"name"`
	CostCenter string `json:"cost_center"`
	Unit       string `json:"unit"` // e.g. "hours", "kilometers"
	Active     *bool  `json:"active"`
	// IsDefault, when set, makes this the customer's only default project.
	// Omit the field to leave the current value unchanged.
	IsDefault *bool `json:"is_default"`
}

// ReportRequest is create/update payload for reports.
// Times are UTC instants: RFC3339 (with Z or an offset) or "2006-01-02 15:04:05"
// with no zone, which is read as UTC. Display conversion happens in the client.
type ReportRequest struct {
	UserID    *int64  `json:"user_id"`
	ProjectID *int64  `json:"project_id"`
	Start     *string `json:"start"`
	Stop      *string `json:"stop"`
	Comment   *string `json:"comment"`
	Deleted   *bool   `json:"deleted"`
}

// MonthlyReportResponse is aggregated monthly time.
type MonthlyReportResponse struct {
	Month      string            `json:"month"`
	UserID     int64             `json:"user_id"`
	CustomerID int64             `json:"customer_id"`
	ProjectID  int64             `json:"project_id"`
	TotalHours float64           `json:"total_hours"`
	TotalHM    string            `json:"total_hm"`
	Customers  []MonthlyCustomer `json:"customers"`
}

type MonthlyCustomer struct {
	CustomerID  int64            `json:"customer_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	TotalHours  float64          `json:"total_hours"`
	TotalHM     string           `json:"total_hm"`
	Invoiced    bool             `json:"invoiced"`
	Days        []MonthlyDay     `json:"days"`
	Projects    []MonthlyProject `json:"projects"`
}

// MonthlyProject is one row in the customer summary. Time is included only
// when the interval names this project alone. Active projects with no such
// time are still listed, at 00:00.
type MonthlyProject struct {
	ProjectID  int64   `json:"project_id"`
	Name       string  `json:"name"`
	CostCenter string  `json:"cost_center"`
	Unit       string  `json:"unit"`
	TotalHours float64 `json:"total_hours"`
	TotalHM    string  `json:"total_hm"`
}

type MonthlyDay struct {
	Date       string         `json:"date"`
	TotalHours float64        `json:"total_hours"`
	TotalHM    string         `json:"total_hm"`
	Entries    []MonthlyEntry `json:"entries"`
	// Projects is this day's billing split. Lengths add up to the day's worked time.
	Projects []MonthlyProject `json:"projects"`
}

// MonthlyNote is one project and the comment written for that project on this
// report. Every report on a day lists every project named anywhere that day.
// A project this report did not mention has an empty comment and no length.
// LengthHM is that project's share of the whole day, set on the first report
// that names the project and empty after that.
type MonthlyNote struct {
	ProjectID int64  `json:"project_id"`
	Project   string `json:"project"`
	Comment   string `json:"comment"`
	LengthHM  string `json:"length_hm,omitempty"`
}

type MonthlyEntry struct {
	ID    int64         `json:"id"`
	Start string        `json:"start"`
	Stop  string        `json:"stop"`
	Hours float64       `json:"hours"`
	HM    string        `json:"hm"`
	Notes []MonthlyNote `json:"notes"`
}

// TimesheetCustomerRow is one customer with time in a month.
type TimesheetCustomerRow struct {
	CustomerID  int64   `json:"customer_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	TotalHours  float64 `json:"total_hours"`
	TotalHM     string  `json:"total_hm"`
	EntryCount  int     `json:"entry_count"`
	Invoiced    bool    `json:"invoiced"`
}

// TimesheetListResponse lists customers with time for a month.
type TimesheetListResponse struct {
	Month     string                 `json:"month"`
	Customers []TimesheetCustomerRow `json:"customers"`
}

// TimesheetInvoiceRequest marks a customer timesheet as invoiced or not.
type TimesheetInvoiceRequest struct {
	CustomerID int64  `json:"customer_id"`
	Month      string `json:"month"` // YYYY-MM
	Invoiced   *bool  `json:"invoiced"`
}

// TimesheetProjectRow is one project's aggregated reported time for a customer/month.
type TimesheetProjectRow struct {
	ProjectID  int64   `json:"project_id"`
	Name       string  `json:"name"`
	CostCenter string  `json:"cost_center"`
	Unit       string  `json:"unit"`
	TotalHours float64 `json:"total_hours"`
	TotalHM    string  `json:"total_hm"`
	TotalSec   int64   `json:"total_sec"`
	EntryCount int     `json:"entry_count"`
	Active     bool    `json:"active"`
}

// TimesheetProjectsResponse lists projects (with totals) for a timesheet dialog.
type TimesheetProjectsResponse struct {
	Month      string                `json:"month"`
	CustomerID int64                 `json:"customer_id"`
	TotalHours float64               `json:"total_hours"`
	TotalHM    string                `json:"total_hm"`
	TotalSec   int64                 `json:"total_sec"`
	Projects   []TimesheetProjectRow `json:"projects"`
}

// TimesheetReallocateRequest moves reported time between projects for one day
// of a customer month. Sum of time is preserved: each move takes duration from
// FromProjectID and gives it to ToProjectID. Only reports that start on Date
// are changed.
type TimesheetReallocateRequest struct {
	CustomerID int64                     `json:"customer_id"`
	Month      string                    `json:"month"` // YYYY-MM
	Date       string                    `json:"date"`  // YYYY-MM-DD, inside Month
	Moves      []TimesheetReallocateMove `json:"moves"`
}

// TimesheetReallocateMove is one transfer of time between projects.
// Prefer Seconds; if zero, HM ("HH:MM" or "H:MM") is parsed.
type TimesheetReallocateMove struct {
	FromProjectID int64  `json:"from_project_id"`
	ToProjectID   int64  `json:"to_project_id"`
	Seconds       int64  `json:"seconds"`
	HM            string `json:"hm"` // optional alternative to seconds
}

// LegacyReport is the desktop sync payload (project_id replaces activityid).
// Lines are the project/comment rows. One line is a normal project report.
// Several lines share the same start and stop.
type LegacyReport struct {
	ID         int64               `json:"_id"`
	UserID     int64               `json:"user_id"`
	CustomerID int64               `json:"customer_id,omitempty"`
	ProjectID  int64               `json:"project_id"`
	Start      time.Time           `json:"start"`
	Stop       *time.Time          `json:"stop"`
	Comment    string              `json:"comment"`
	Lines      []models.ReportLine `json:"lines,omitempty"`
	Modified   time.Time           `json:"modified"`
	Seq        int64               `json:"seq"`
	Deleted    int                 `json:"deleted"`
	ClientUUID string              `json:"client_uuid,omitempty"`
}

func LegacyReportFrom(r *models.Report) LegacyReport {
	del := 0
	if r.Deleted {
		del = 1
	}
	cu := ""
	if r.ClientUUID != nil {
		cu = *r.ClientUUID
	}
	return LegacyReport{
		ID:         r.ID,
		UserID:     r.UserID,
		CustomerID: r.CustomerID,
		ProjectID:  r.ProjectID,
		Start:      r.Start,
		Stop:       r.Stop,
		Comment:    r.Comment,
		Lines:      r.Lines,
		Modified:   r.Modified,
		Seq:        r.Seq,
		Deleted:    del,
		ClientUUID: cu,
	}
}

// ParseTime reads a report instant. A value with Z or a numeric offset is that
// absolute instant. A value with no zone is UTC, so rows from different
// clients stay comparable. The result is always in time.UTC.
func ParseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999",
		"2006-01-02T15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
	} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", s)
}
