package handlers

import (
	"fmt"
	"time"

	"github.com/abundo/ergotime-server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// dayShares is "user|customer|YYYY-MM-DD" → project_id → seconds.
type dayShares map[string]map[int64]int64

func durationSeconds(r models.Report) int64 {
	if r.Stop == nil {
		return 0
	}
	sec := int64(r.Stop.Sub(r.Start) / time.Second)
	if sec < 0 {
		return 0
	}
	return sec
}

func anchorProjectID(r models.Report) int64 {
	if r.ProjectID > 0 {
		return r.ProjectID
	}
	if len(r.Lines) > 0 {
		return r.Lines[0].ProjectID
	}
	return 0
}

func dayShareKey(userID, customerID int64, day string) string {
	return fmt.Sprintf("%d|%d|%s", userID, customerID, day)
}

func reportDayKey(r models.Report, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return dayShareKey(r.UserID, r.CustomerID, r.Start.In(loc).Format("2006-01-02"))
}

func sumShareSeconds(shares map[int64]int64) int64 {
	var sum int64
	for _, sec := range shares {
		if sec > 0 {
			sum += sec
		}
	}
	return sum
}

func positiveShares(shares map[int64]int64) map[int64]int64 {
	out := make(map[int64]int64, len(shares))
	for id, sec := range shares {
		if sec > 0 {
			out[id] = sec
		}
	}
	return out
}

// defaultDayBilling bills each report on its own project.
func defaultDayBilling(reports []models.Report) (map[int64]int64, int64) {
	out := map[int64]int64{}
	var total int64
	for _, r := range reports {
		sec := durationSeconds(r)
		if sec <= 0 {
			continue
		}
		total += sec
		anchor := anchorProjectID(r)
		if anchor > 0 {
			out[anchor] += sec
		}
	}
	return out, total
}

// dayBilling is the billing split for one user's customer-day.
// Stored amounts are used only when they add up to the worked time.
func dayBilling(reports []models.Report, stored map[int64]int64) map[int64]int64 {
	base, total := defaultDayBilling(reports)
	if total > 0 && sumShareSeconds(stored) == total {
		return positiveShares(stored)
	}
	return base
}

func sameShareMap(a, b map[int64]int64) bool {
	if len(positiveShares(a)) != len(positiveShares(b)) {
		return false
	}
	for id, sec := range positiveShares(a) {
		if b[id] != sec {
			return false
		}
	}
	return true
}

func projectsNamedOnReports(reports []models.Report) map[int64]bool {
	out := map[int64]bool{}
	for _, r := range reports {
		if r.ProjectID > 0 && len(r.Lines) == 0 {
			out[r.ProjectID] = true
		}
		for _, ln := range r.Lines {
			if ln.ProjectID > 0 {
				out[ln.ProjectID] = true
			}
		}
		if len(r.Lines) == 0 && r.ProjectID > 0 {
			continue
		}
		if anchor := anchorProjectID(r); anchor > 0 && len(r.Lines) == 0 {
			out[anchor] = true
		}
	}
	return out
}

func loadDayShares(db *gorm.DB, userIDs, customerIDs []int64) (dayShares, error) {
	out := dayShares{}
	if db == nil || len(userIDs) == 0 || len(customerIDs) == 0 {
		return out, nil
	}
	var rows []models.DayShare
	if err := db.Where("user_id IN ? AND customer_id IN ?", userIDs, customerIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := dayShareKey(row.UserID, row.CustomerID, row.Day)
		if out[key] == nil {
			out[key] = map[int64]int64{}
		}
		out[key][row.ProjectID] += row.Seconds
	}
	return out, nil
}

func distinctIDs(reports []models.Report, pick func(models.Report) int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, r := range reports {
		id := pick(r)
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// planDayShareMove moves wantSec from one project to another within one day.
// The returned map is the whole day's billing. Report rows are not changed.
func planDayShareMove(current map[int64]int64, fromID, toID, wantSec int64) (map[int64]int64, error) {
	if wantSec <= 0 {
		return nil, nil
	}
	have := current[fromID]
	if have < wantSec {
		return nil, &badRequestError{
			msg: fmt.Sprintf("not enough time on project %d: have %s, need %s",
				fromID, formatHM(time.Duration(have)*time.Second), formatHM(time.Duration(wantSec)*time.Second)),
		}
	}
	next := make(map[int64]int64, len(current)+1)
	for id, sec := range current {
		if sec > 0 {
			next[id] = sec
		}
	}
	next[fromID] = have - wantSec
	if next[fromID] <= 0 {
		delete(next, fromID)
	}
	next[toID] += wantSec
	return next, nil
}

func replaceDayShares(tx *gorm.DB, userID, customerID int64, day string, shares map[int64]int64, reports []models.Report) error {
	if err := tx.Where("user_id = ? AND customer_id = ? AND day = ?", userID, customerID, day).
		Delete(&models.DayShare{}).Error; err != nil {
		return err
	}
	base, _ := defaultDayBilling(reports)
	if sameShareMap(shares, base) {
		return nil
	}
	rows := make([]models.DayShare, 0, len(shares))
	for pid, sec := range shares {
		if pid <= 0 || sec <= 0 {
			continue
		}
		rows = append(rows, models.DayShare{
			UserID: userID, CustomerID: customerID, Day: day, ProjectID: pid, Seconds: sec,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// moveTimeBetweenProjects records a billing split for one user's customer-day.
// Start, stop, project, and comments stay as written. The day's projects share
// the sum of that day's report lengths.
func moveTimeBetweenProjects(
	tx *gorm.DB,
	userID, customerID int64,
	day string,
	start, end time.Time,
	fromID, toID, wantSec int64,
) error {
	if wantSec <= 0 {
		return nil
	}
	var reports []models.Report
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			`user_id = ? AND customer_id = ? AND deleted = ? AND stop IS NOT NULL AND start >= ? AND start < ?`,
			userID, customerID, false, start, end,
		).
		Order("start ASC, _id ASC").
		Find(&reports).Error
	if err != nil {
		return fmt.Errorf("load reports: %w", err)
	}
	if err := attachLines(tx, reports); err != nil {
		return fmt.Errorf("load lines: %w", err)
	}
	named := projectsNamedOnReports(reports)
	if !named[fromID] || !named[toID] {
		return &badRequestError{msg: "both projects must be on this day"}
	}
	if len(named) < 2 {
		return &badRequestError{msg: "this day has only one project"}
	}
	stored, err := loadDayShares(tx, []int64{userID}, []int64{customerID})
	if err != nil {
		return fmt.Errorf("load shares: %w", err)
	}
	current := dayBilling(reports, stored[dayShareKey(userID, customerID, day)])
	next, err := planDayShareMove(current, fromID, toID, wantSec)
	if err != nil {
		return err
	}
	if err := replaceDayShares(tx, userID, customerID, day, next, reports); err != nil {
		return fmt.Errorf("save shares: %w", err)
	}
	return nil
}
