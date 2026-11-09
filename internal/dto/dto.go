package dto

import (
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
}

func UserFromModel(u *models.User) UserPublic {
	return UserPublic{
		ID:       u.ID,
		Name:     u.Name,
		FullName: u.FullName,
		Active:   u.Active,
		IsAdmin:  u.IsAdmin,
	}
}

// CreateUserRequest creates a new local user.
type CreateUserRequest struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
	Active   *bool  `json:"active"`
	IsAdmin  *bool  `json:"is_admin"`
}

// UpdateUserRequest updates user fields (password optional).
type UpdateUserRequest struct {
	Name     *string `json:"name"`
	FullName *string `json:"full_name"`
	Password *string `json:"password"`
	Active   *bool   `json:"active"`
	IsAdmin  *bool   `json:"is_admin"`
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
}

// ReportRequest is create/update payload for reports.
// Times are RFC3339 or "2006-01-02 15:04:05".
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
	Projects    []MonthlyProject `json:"projects"`
}

type MonthlyProject struct {
	ProjectID  int64        `json:"project_id"`
	Name       string       `json:"name"`
	CostCenter string       `json:"cost_center"`
	TotalHours float64      `json:"total_hours"`
	TotalHM    string       `json:"total_hm"`
	Days       []MonthlyDay `json:"days"`
}

type MonthlyDay struct {
	Date       string         `json:"date"`
	TotalHours float64        `json:"total_hours"`
	TotalHM    string         `json:"total_hm"`
	Entries    []MonthlyEntry `json:"entries"`
}

type MonthlyEntry struct {
	ID      int64   `json:"id"`
	Start   string  `json:"start"`
	Stop    string  `json:"stop"`
	Hours   float64 `json:"hours"`
	HM      string  `json:"hm"`
	Comment string  `json:"comment"`
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

// TimesheetReallocateRequest moves reported time between projects for one customer/month.
// Sum of time is preserved: each move takes duration from FromProjectID and gives it to ToProjectID.
type TimesheetReallocateRequest struct {
	CustomerID int64                     `json:"customer_id"`
	Month      string                    `json:"month"` // YYYY-MM
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
type LegacyReport struct {
	ID         int64      `json:"_id"`
	UserID     int64      `json:"user_id"`
	ProjectID  int64      `json:"project_id"`
	Start      time.Time  `json:"start"`
	Stop       *time.Time `json:"stop"`
	Comment    string     `json:"comment"`
	Modified   time.Time  `json:"modified"`
	Seq        int64      `json:"seq"`
	Deleted    int        `json:"deleted"`
	ClientUUID string     `json:"client_uuid,omitempty"`
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
		ProjectID:  r.ProjectID,
		Start:      r.Start,
		Stop:       r.Stop,
		Comment:    r.Comment,
		Modified:   r.Modified,
		Seq:        r.Seq,
		Deleted:    del,
		ClientUUID: cu,
	}
}

// Time layouts accepted from clients.
var TimeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999",
}

func ParseTime(s string) (time.Time, error) {
	var last error
	for _, layout := range TimeLayouts {
		t, err := time.ParseInLocation(layout, s, time.Local)
		if err == nil {
			return t, nil
		}
		last = err
	}
	return time.Time{}, last
}
