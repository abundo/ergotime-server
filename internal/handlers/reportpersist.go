package handlers

import (
	"errors"
	"fmt"

	"github.com/abundo/ergotime-server/internal/models"
	"gorm.io/gorm"
)

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
	return replaceLines(tx, item.ID, item.Lines)
}

func lineError(err error) (string, bool) {
	switch {
	case errors.Is(err, models.ErrNoReportLine):
		return "project_id is required", true
	case errors.Is(err, models.ErrUnknownProject):
		return "project not found", true
	case errors.Is(err, models.ErrMixedCustomer):
		return err.Error(), true
	default:
		return "", false
	}
}

func retargetReportLine(tx *gorm.DB, reportID, projectID int64, comment string) error {
	var n int64
	if err := tx.Model(&models.ReportLine{}).Where("report_id = ?", reportID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return tx.Create(&models.ReportLine{
			ReportID:  reportID,
			ProjectID: projectID,
			Comment:   comment,
			Position:  0,
		}).Error
	}
	return tx.Model(&models.ReportLine{}).Where("report_id = ?", reportID).Updates(map[string]any{
		"project_id": projectID,
		"comment":    comment,
		"position":   0,
	}).Error
}

// singleLineSQL excludes reports whose time is shared by several project lines.
func singleLineSQL(column string) string {
	return fmt.Sprintf(`%s NOT IN (
		SELECT report_id FROM report_line GROUP BY report_id HAVING COUNT(*) > 1
	)`, column)
}
