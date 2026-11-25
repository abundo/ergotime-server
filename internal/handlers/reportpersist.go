package handlers

import (
	"errors"
	"time"

	"github.com/abundo/ergotime-server/internal/models"
	"gorm.io/gorm"
)

// Closed reports start and stop on a minute boundary (second and fraction 0)
// and last at least one minute. Stop is the next whole minute, so 12:20 for
// one minute is 12:20:00–12:21:00. An open report may omit stop; its start
// still has to be a whole minute. :59 is not a boundary.
var (
	errStartNotMinute = errors.New("start must be on a minute boundary")
	errStopNotMinute  = errors.New("stop must be on a minute boundary")
	errReportTooShort = errors.New("report must be at least 1 minute")
)

func onMinute(t time.Time) bool {
	return t.Second() == 0 && t.Nanosecond() == 0
}

func validateReportTimes(start time.Time, stop *time.Time) error {
	if !onMinute(start) {
		return errStartNotMinute
	}
	if stop == nil {
		return nil
	}
	if !onMinute(*stop) {
		return errStopNotMinute
	}
	if stop.Sub(start) < time.Minute {
		return errReportTooShort
	}
	return nil
}

func attachLines(db *gorm.DB, items []models.Report) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for i := range items {
		if items[i].ID > 0 {
			ids = append(ids, items[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var lines []models.ReportLine
	if err := db.Where("report_id IN ?", ids).Order("position ASC, _id ASC").Find(&lines).Error; err != nil {
		return err
	}
	byReport := map[int64][]models.ReportLine{}
	for _, ln := range lines {
		byReport[ln.ReportID] = append(byReport[ln.ReportID], ln)
	}
	for i := range items {
		items[i].Lines = byReport[items[i].ID]
	}
	return nil
}

func replaceLines(tx *gorm.DB, reportID int64, lines []models.ReportLine) error {
	if err := tx.Where("report_id = ?", reportID).Delete(&models.ReportLine{}).Error; err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	for i := range lines {
		lines[i].ID = 0
		lines[i].ReportID = reportID
		lines[i].Position = i
	}
	return tx.Create(&lines).Error
}

// applyLines sets customer, anchor project, comment, and lines from the request.
// When LinesSet is false, the single project_id and comment are the only line.
func applyLines(tx *gorm.DB, item *models.Report) error {
	inputs := make([]models.LineInput, 0, len(item.Lines))
	if item.LinesSet {
		for _, ln := range item.Lines {
			inputs = append(inputs, models.LineInput{ProjectID: ln.ProjectID, Comment: ln.Comment})
		}
	} else if item.ProjectID > 0 {
		inputs = append(inputs, models.LineInput{ProjectID: item.ProjectID, Comment: item.Comment})
	}
	if len(inputs) == 0 {
		return models.ErrNoReportLine
	}
	ids := make([]int64, 0, len(inputs))
	seen := map[int64]bool{}
	for _, in := range inputs {
		if in.ProjectID > 0 && !seen[in.ProjectID] {
			seen[in.ProjectID] = true
			ids = append(ids, in.ProjectID)
		}
	}
	var projects []models.Project
	if len(ids) > 0 {
		if err := tx.Where("_id IN ?", ids).Find(&projects).Error; err != nil {
			return err
		}
	}
	customerID, anchor, comment, lines, err := models.NormalizeReportLines(projects, inputs, item.ProjectID)
	if err != nil {
		return err
	}
	item.CustomerID = customerID
	item.ProjectID = anchor
	item.Comment = comment
	item.Lines = lines
	return nil
}

func writeReport(tx *gorm.DB, item *models.Report, isNew bool) error {
	if err := applyLines(tx, item); err != nil {
		return err
	}
	// Instants are stored as UTC. timestamptz keeps the instant; the location
	// on the Go value is what later JSON and day-bucketing start from.
	item.Start = item.Start.UTC()
	if item.Stop != nil {
		stop := item.Stop.UTC()
		item.Stop = &stop
	}
	if err := validateReportTimes(item.Start, item.Stop); err != nil {
		return err
	}
	if !item.Modified.IsZero() {
		item.Modified = item.Modified.UTC()
	}
	if isNew {
		if err := tx.Create(item).Error; err != nil {
			return err
		}
	} else if err := tx.Save(item).Error; err != nil {
		return err
	}
	if err := replaceLines(tx, item.ID, item.Lines); err != nil {
		return err
	}
	// Editing the worked interval drops that day's billing split. The whole
	// day is billed on each report's own project again until time is moved.
	return invalidateDayShares(tx, item)
}

func invalidateDayShares(tx *gorm.DB, item *models.Report) error {
	if item.UserID == 0 || item.CustomerID == 0 || item.Start.IsZero() {
		return nil
	}
	days := []string{item.Start.UTC().Format("2006-01-02")}
	var tz string
	_ = tx.Model(&models.User{}).Where("_id = ?", item.UserID).Pluck("timezone", &tz).Error
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			local := item.Start.In(loc).Format("2006-01-02")
			if local != days[0] {
				days = append(days, local)
			}
		}
	}
	return tx.Where("user_id = ? AND customer_id = ? AND day IN ?", item.UserID, item.CustomerID, days).
		Delete(&models.DayShare{}).Error
}

func lineError(err error) (string, bool) {
	switch {
	case errors.Is(err, models.ErrNoReportLine):
		return "project_id is required", true
	case errors.Is(err, models.ErrUnknownProject):
		return "project not found", true
	case errors.Is(err, models.ErrMixedCustomer):
		return err.Error(), true
	case errors.Is(err, errStartNotMinute), errors.Is(err, errStopNotMinute), errors.Is(err, errReportTooShort):
		return err.Error(), true
	default:
		return "", false
	}
}
