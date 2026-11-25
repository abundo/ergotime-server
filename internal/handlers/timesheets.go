package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abundo/ergotime-server/internal/dto"
	"github.com/abundo/ergotime-server/internal/middleware"
	"github.com/abundo/ergotime-server/internal/models"
	"github.com/go-pdf/fpdf"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// TimesheetsHandler manages customer timesheet listing, PDF export, and invoice status.
type TimesheetsHandler struct {
	DB *gorm.DB
}

// timesheetNote is one project and the comment written for that project.
// Several notes share one worked interval. Length is that project's share of
// the whole day when this row shows it, and empty on the other rows.
type timesheetNote struct {
	ProjectID int64
	Project   string
	Comment   string
	Length    string
}

type timesheetDayEntry struct {
	ID       int64
	Start    string
	Stop     string
	Length   string
	Duration time.Duration
	Notes    []timesheetNote
}

type timesheetDay struct {
	Date    string
	Entries []timesheetDayEntry
	Total   time.Duration
}

// timesheetProjectSummary is one project's aggregated reported time for a timesheet.
// Unit is a free-text label from the project (hours, kilometers, …); not interpreted.
type timesheetProjectSummary struct {
	ProjectID int64
	Name      string
	Unit      string
	Duration  time.Duration
	Entries   int
}

// List returns customers that have reported time in the given month.
// GET /api/timesheets?month=YYYY-MM
func (h *TimesheetsHandler) List(c echo.Context) error {
	monthStr, start, end, _, err := h.monthRange(c, c.QueryParam("month"))
	if err != nil {
		return badRequest(c, err.Error())
	}

	type custAgg struct {
		CustomerID  int64
		TotalSec    float64
		EntryCount  int
		AllInvoiced bool
	}
	var aggs []custAgg
	// Join report → project to group by customer.
	err = h.DB.Table("report r").
		Select(`p.customer_id AS customer_id,
			COALESCE(SUM(EXTRACT(EPOCH FROM (r.stop - r.start))), 0) AS total_sec,
			COUNT(*) AS entry_count,
			BOOL_AND(r.invoiced) AS all_invoiced`).
		Joins("JOIN project p ON p._id = r.project_id").
		Where("r.deleted = ? AND r.stop IS NOT NULL AND r.start >= ? AND r.start < ?", false, start, end).
		Group("p.customer_id").
		Having("COUNT(*) > 0").
		Scan(&aggs).Error
	if err != nil {
		return internal(c, "failed to aggregate timesheets")
	}

	if len(aggs) == 0 {
		return ok(c, dto.TimesheetListResponse{Month: monthStr, Customers: []dto.TimesheetCustomerRow{}})
	}

	ids := make([]int64, 0, len(aggs))
	aggByID := make(map[int64]custAgg, len(aggs))
	for _, a := range aggs {
		ids = append(ids, a.CustomerID)
		aggByID[a.CustomerID] = a
	}

	var customers []models.Customer
	if err := h.DB.Where("_id IN ?", ids).Order("name ASC").Find(&customers).Error; err != nil {
		return internal(c, "failed to load customers")
	}

	rows := make([]dto.TimesheetCustomerRow, 0, len(customers))
	for _, cu := range customers {
		agg := aggByID[cu.ID]
		dur := time.Duration(agg.TotalSec * float64(time.Second))
		rows = append(rows, dto.TimesheetCustomerRow{
			CustomerID:  cu.ID,
			Name:        cu.Name,
			Description: cu.Description,
			TotalHours:  dur.Hours(),
			TotalHM:     formatHM(dur),
			EntryCount:  agg.EntryCount,
			Invoiced:    agg.AllInvoiced,
		})
	}

	return ok(c, dto.TimesheetListResponse{Month: monthStr, Customers: rows})
}

// MarkInvoiced sets or clears the invoiced flag on every closed report for a
// customer in the given month.
// PUT /api/timesheets/invoiced
// Body: { "customer_id": N, "month": "YYYY-MM", "invoiced": true|false }
func (h *TimesheetsHandler) MarkInvoiced(c echo.Context) error {
	var req dto.TimesheetInvoiceRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	if req.CustomerID <= 0 {
		return badRequest(c, "customer_id is required")
	}
	monthStr, start, end, _, err := h.monthRange(c, req.Month)
	if err != nil {
		return badRequest(c, err.Error())
	}
	invoiced := true
	if req.Invoiced != nil {
		invoiced = *req.Invoiced
	}

	var cust models.Customer
	if err := h.DB.First(&cust, req.CustomerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "customer not found")
		}
		return internal(c, "database error")
	}

	res := h.DB.Model(&models.Report{}).
		Where(
			`project_id IN (SELECT _id FROM project WHERE customer_id = ?)
			 AND deleted = ? AND stop IS NOT NULL AND start >= ? AND start < ?`,
			req.CustomerID, false, start, end,
		).
		Update("invoiced", invoiced)
	if res.Error != nil {
		return internal(c, "failed to update invoice status")
	}

	return ok(c, map[string]any{
		"customer_id":  req.CustomerID,
		"month":        monthStr,
		"invoiced":     invoiced,
		"updated_rows": res.RowsAffected,
	})
}

// Projects returns per-project totals for a customer/month (timesheet dialog).
// Includes active customer projects with zero time so time can be moved onto them.
// GET /api/timesheets/projects?month=YYYY-MM&customer_id=N
func (h *TimesheetsHandler) Projects(c echo.Context) error {
	monthStr, start, end, _, err := h.monthRange(c, c.QueryParam("month"))
	if err != nil {
		return badRequest(c, err.Error())
	}
	customerID, err := strconv.ParseInt(c.QueryParam("customer_id"), 10, 64)
	if err != nil || customerID <= 0 {
		return badRequest(c, "customer_id is required")
	}

	var cust models.Customer
	if err := h.DB.First(&cust, customerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "customer not found")
		}
		return internal(c, "database error")
	}

	resp, err := h.buildTimesheetProjects(customerID, monthStr, start, end)
	if err != nil {
		return internal(c, "failed to load project totals")
	}
	return ok(c, resp)
}

func (h *TimesheetsHandler) buildTimesheetProjects(
	customerID int64,
	monthStr string,
	start, end time.Time,
) (dto.TimesheetProjectsResponse, error) {
	var projects []models.Project
	if err := h.DB.Where("customer_id = ?", customerID).Order("name ASC").Find(&projects).Error; err != nil {
		return dto.TimesheetProjectsResponse{}, err
	}

	var reports []models.Report
	err := h.DB.Where(
		`project_id IN (SELECT _id FROM project WHERE customer_id = ?)
		 AND deleted = ? AND stop IS NOT NULL AND start >= ? AND start < ?`,
		customerID, false, start, end,
	).Find(&reports).Error
	if err != nil {
		return dto.TimesheetProjectsResponse{}, err
	}

	type agg struct {
		sec   int64
		count int
	}
	if err := attachLines(h.DB, reports); err != nil {
		return dto.TimesheetProjectsResponse{}, err
	}
	userIDs := distinctIDs(reports, func(r models.Report) int64 { return r.UserID })
	shares, err := loadDayShares(h.DB, userIDs, []int64{customerID})
	if err != nil {
		return dto.TimesheetProjectsResponse{}, err
	}

	byProj := map[int64]*agg{}
	var grandSec int64
	grouped := map[string][]models.Report{}
	for _, r := range reports {
		grandSec += durationSeconds(r)
		if durationSeconds(r) <= 0 {
			continue
		}
		key := reportDayKey(r, time.UTC)
		grouped[key] = append(grouped[key], r)
	}
	for key, dayReports := range grouped {
		for projectID, sec := range dayBilling(dayReports, shares[key]) {
			a := byProj[projectID]
			if a == nil {
				a = &agg{}
				byProj[projectID] = a
			}
			a.sec += sec
			a.count++
		}
	}

	// Show projects that have time, plus all active projects (targets for moves).
	rows := make([]dto.TimesheetProjectRow, 0, len(projects))
	seen := map[int64]bool{}
	for _, p := range projects {
		a := byProj[p.ID]
		if a == nil && !p.Active {
			continue
		}
		sec := int64(0)
		count := 0
		if a != nil {
			sec = a.sec
			count = a.count
		}
		unit := strings.TrimSpace(p.Unit)
		if unit == "" {
			unit = "hours"
		}
		dur := time.Duration(sec) * time.Second
		rows = append(rows, dto.TimesheetProjectRow{
			ProjectID:  p.ID,
			Name:       p.Name,
			CostCenter: p.CostCenter,
			Unit:       unit,
			TotalHours: dur.Hours(),
			TotalHM:    formatHM(dur),
			TotalSec:   sec,
			EntryCount: count,
			Active:     p.Active,
		})
		seen[p.ID] = true
	}
	// Orphan project ids (deleted project with leftover reports) — still show.
	for pid, a := range byProj {
		if seen[pid] {
			continue
		}
		dur := time.Duration(a.sec) * time.Second
		rows = append(rows, dto.TimesheetProjectRow{
			ProjectID:  pid,
			Name:       fmt.Sprintf("Project #%d", pid),
			Unit:       "hours",
			TotalHours: dur.Hours(),
			TotalHM:    formatHM(dur),
			TotalSec:   a.sec,
			EntryCount: a.count,
			Active:     false,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name == rows[j].Name {
			return rows[i].ProjectID < rows[j].ProjectID
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	grand := time.Duration(grandSec) * time.Second
	return dto.TimesheetProjectsResponse{
		Month:      monthStr,
		CustomerID: customerID,
		TotalHours: grand.Hours(),
		TotalHM:    formatHM(grand),
		TotalSec:   grandSec,
		Projects:   rows,
	}, nil
}

// Reallocate moves reported time between projects for one day of a customer month.
// The day's total, and the month total, stay the same (only project assignment changes).
// PUT /api/timesheets/reallocate
func (h *TimesheetsHandler) Reallocate(c echo.Context) error {
	var req dto.TimesheetReallocateRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	if req.CustomerID <= 0 {
		return badRequest(c, "customer_id is required")
	}
	if len(req.Moves) == 0 {
		return badRequest(c, "moves is required")
	}
	monthStr, start, end, loc, err := h.monthRange(c, req.Month)
	if err != nil {
		return badRequest(c, err.Error())
	}
	dayStart, dayEnd, err := dayWindowInMonth(req.Date, monthStr, loc)
	if err != nil {
		return badRequest(c, err.Error())
	}

	var cust models.Customer
	if err := h.DB.First(&cust, req.CustomerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "customer not found")
		}
		return internal(c, "database error")
	}

	// All customer project ids for validation.
	var projects []models.Project
	if err := h.DB.Where("customer_id = ?", req.CustomerID).Find(&projects).Error; err != nil {
		return internal(c, "failed to load projects")
	}
	projOK := map[int64]bool{}
	for _, p := range projects {
		projOK[p.ID] = true
	}

	type move struct {
		from, to int64
		sec      int64
	}
	moves := make([]move, 0, len(req.Moves))
	for i, m := range req.Moves {
		sec := m.Seconds
		if sec <= 0 && strings.TrimSpace(m.HM) != "" {
			parsed, err := parseHMDuration(m.HM)
			if err != nil {
				return badRequest(c, fmt.Sprintf("moves[%d]: invalid hm %q", i, m.HM))
			}
			sec = int64(parsed / time.Second)
		}
		if sec <= 0 {
			return badRequest(c, fmt.Sprintf("moves[%d]: seconds or hm must be positive", i))
		}
		if m.FromProjectID <= 0 || m.ToProjectID <= 0 {
			return badRequest(c, fmt.Sprintf("moves[%d]: from_project_id and to_project_id are required", i))
		}
		if m.FromProjectID == m.ToProjectID {
			return badRequest(c, fmt.Sprintf("moves[%d]: choose a different project to move the time to", i))
		}
		if !projOK[m.FromProjectID] || !projOK[m.ToProjectID] {
			return badRequest(c, fmt.Sprintf("moves[%d]: projects must belong to the customer", i))
		}
		moves = append(moves, move{from: m.FromProjectID, to: m.ToProjectID, sec: sec})
	}

	// Refuse when any entry is already invoiced.
	var invoicedCount int64
	if err := h.DB.Model(&models.Report{}).
		Where(
			`project_id IN (SELECT _id FROM project WHERE customer_id = ?)
			 AND deleted = ? AND stop IS NOT NULL AND start >= ? AND start < ? AND invoiced = ?`,
			req.CustomerID, false, start, end, true,
		).Count(&invoicedCount).Error; err != nil {
		return internal(c, "database error")
	}
	if invoicedCount > 0 {
		return badRequest(c, "cannot reallocate: timesheet has invoiced entries (unmark invoiced first)")
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		userID := middleware.UserIDFromContext(c)
		for _, mv := range moves {
			if err := moveTimeBetweenProjects(tx, userID, req.CustomerID, dayStart.Format("2006-01-02"), dayStart, dayEnd, mv.from, mv.to, mv.sec); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		var bad *badRequestError
		if errors.As(err, &bad) {
			return badRequest(c, bad.msg)
		}
		return internal(c, "failed to reallocate time")
	}

	resp, err := h.buildTimesheetProjects(req.CustomerID, monthStr, start, end)
	if err != nil {
		return internal(c, "failed to load project totals")
	}
	return ok(c, resp)
}

type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }

// PDF generates a timesheet PDF for one customer and month.
// GET /api/timesheets/pdf?month=YYYY-MM&customer_id=N
func (h *TimesheetsHandler) PDF(c echo.Context) error {
	monthStr, start, end, loc, err := h.monthRange(c, c.QueryParam("month"))
	if err != nil {
		return badRequest(c, err.Error())
	}
	customerID, err := strconv.ParseInt(c.QueryParam("customer_id"), 10, 64)
	if err != nil || customerID <= 0 {
		return badRequest(c, "customer_id is required")
	}

	var cust models.Customer
	if err := h.DB.First(&cust, customerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "customer not found")
		}
		return internal(c, "database error")
	}

	var reports []models.Report
	err = h.DB.Where(
		`(customer_id = ? OR project_id IN (SELECT _id FROM project WHERE customer_id = ?))
		 AND deleted = ? AND stop IS NOT NULL AND start >= ? AND start < ?`,
		customerID, customerID, false, start, end,
	).Order("start ASC").Find(&reports).Error
	if err != nil {
		return internal(c, "failed to load reports")
	}
	if err := attachLines(h.DB, reports); err != nil {
		return internal(c, "failed to load report lines")
	}

	// Load projects referenced by reports (names for the project column and the summary).
	projIDs := map[int64]struct{}{}
	for _, r := range reports {
		if len(r.Lines) == 0 {
			projIDs[r.ProjectID] = struct{}{}
			continue
		}
		for _, ln := range r.Lines {
			projIDs[ln.ProjectID] = struct{}{}
		}
	}
	projectsByID := map[int64]models.Project{}
	if len(projIDs) > 0 {
		ids := make([]int64, 0, len(projIDs))
		for id := range projIDs {
			ids = append(ids, id)
		}
		var projects []models.Project
		if err := h.DB.Where("_id IN ?", ids).Find(&projects).Error; err != nil {
			return internal(c, "failed to load projects")
		}
		for _, p := range projects {
			projectsByID[p.ID] = p
		}
	}

	var catalog []models.Project
	if err := h.DB.Where("customer_id = ?", customerID).Find(&catalog).Error; err != nil {
		return internal(c, "failed to load projects")
	}
	for _, p := range catalog {
		projectsByID[p.ID] = p
	}

	names := map[int64]string{}
	for id, p := range projectsByID {
		names[id] = p.Name
	}

	days, grandTotal := groupReportsByDay(reports, names, loc)
	userIDs := distinctIDs(reports, func(r models.Report) int64 { return r.UserID })
	shares, err := loadDayShares(h.DB, userIDs, []int64{customerID})
	if err != nil {
		return internal(c, "failed to load billing splits")
	}
	days = daysWithSharedProjects(days, reports, projectsByID, customerID, loc, shares)
	projectSummaries := projectSummariesForCustomer(reports, projectsByID, catalog, loc, shares)
	periodLabel := start.Format("January 2006")
	customerLabel := strings.TrimSpace(cust.Description)
	if customerLabel == "" {
		customerLabel = cust.Name
	}
	pdfBytes, err := generateTimesheetPDF(customerLabel, periodLabel, days, grandTotal, projectSummaries)
	if err != nil {
		return internal(c, "failed to generate PDF")
	}

	safeName := sanitizeFilename(customerLabel)
	filename := fmt.Sprintf("timesheet-%s-%s.pdf", safeName, monthStr)

	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.Blob(http.StatusOK, "application/pdf", pdfBytes)
}

func projectDisplayName(id int64, names map[int64]string) string {
	if name := strings.TrimSpace(names[id]); name != "" {
		return name
	}
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("Project #%d", id)
}

// notesForReport is one row per project on the interval. Each row keeps that
// project's own comment. A report with no lines falls back to the report comment.
func notesForReport(r models.Report, names map[int64]string) []timesheetNote {
	if len(r.Lines) == 0 {
		return []timesheetNote{{
			ProjectID: r.ProjectID,
			Project:   projectDisplayName(r.ProjectID, names),
			Comment:   r.Comment,
		}}
	}
	notes := make([]timesheetNote, 0, len(r.Lines))
	for _, ln := range r.Lines {
		notes = append(notes, timesheetNote{
			ProjectID: ln.ProjectID,
			Project:   projectDisplayName(ln.ProjectID, names),
			Comment:   ln.Comment,
		})
	}
	return notes
}

func groupReportsByDay(reports []models.Report, names map[int64]string, loc *time.Location) ([]timesheetDay, time.Duration) {
	if loc == nil {
		loc = time.UTC
	}
	var days []timesheetDay
	var cur *timesheetDay
	var grandTotal time.Duration

	for _, r := range reports {
		if r.Stop == nil {
			continue
		}
		dur := r.Stop.Sub(r.Start)
		if dur < 0 {
			continue
		}
		dateStr := r.Start.In(loc).Format("2006-01-02")
		if cur == nil || cur.Date != dateStr {
			if cur != nil {
				days = append(days, *cur)
			}
			cur = &timesheetDay{Date: dateStr}
		}
		cur.Entries = append(cur.Entries, timesheetDayEntry{
			ID:       r.ID,
			Start:    r.Start.In(loc).Format("15:04"),
			Stop:     r.Stop.In(loc).Format("15:04"),
			Length:   formatHM(dur),
			Duration: dur,
			Notes:    notesForReport(r, names),
		})
		cur.Total += dur
		grandTotal += dur
	}
	if cur != nil {
		days = append(days, *cur)
	}
	return days, grandTotal
}

// daysWithSharedProjects lists every project named on a day on each report,
// and puts that project's day length on the first report that names it.
// The same notes are what the monthly report shows.
func daysWithSharedProjects(
	days []timesheetDay,
	reports []models.Report,
	projectsByID map[int64]models.Project,
	customerID int64,
	loc *time.Location,
	shares dayShares,
) []timesheetDay {
	built := monthlyDays(days, reportsByLocalDate(reports, loc), projectsByID, customerID, shares)
	if len(built) != len(days) {
		return days
	}
	out := make([]timesheetDay, len(days))
	for i, day := range days {
		out[i] = day
		out[i].Entries = make([]timesheetDayEntry, len(day.Entries))
		for j, e := range day.Entries {
			out[i].Entries[j] = e
			if j >= len(built[i].Entries) {
				continue
			}
			notes := built[i].Entries[j].Notes
			stamped := make([]timesheetNote, len(notes))
			for k, n := range notes {
				stamped[k] = timesheetNote{
					ProjectID: n.ProjectID,
					Project:   n.Project,
					Comment:   n.Comment,
					Length:    n.LengthHM,
				}
			}
			out[i].Entries[j].Notes = stamped
		}
	}
	return out
}

// summarizeReportsByProject splits reported time for a timesheet into per-project
// totals. Unit is taken from the project (label only). Sorted by project name.
func summarizeReportsByProject(
	reports []models.Report,
	projectsByID map[int64]models.Project,
	loc *time.Location,
	shares dayShares,
) []timesheetProjectSummary {
	if loc == nil {
		loc = time.UTC
	}
	type acc struct {
		duration time.Duration
		entries  int
	}
	grouped := map[string][]models.Report{}
	for _, r := range reports {
		if durationSeconds(r) <= 0 {
			continue
		}
		key := reportDayKey(r, loc)
		grouped[key] = append(grouped[key], r)
	}
	byID := map[int64]*acc{}
	for key, dayReports := range grouped {
		for projectID, sec := range dayBilling(dayReports, shares[key]) {
			a := byID[projectID]
			if a == nil {
				a = &acc{}
				byID[projectID] = a
			}
			a.duration += time.Duration(sec) * time.Second
			a.entries++
		}
	}

	out := make([]timesheetProjectSummary, 0, len(byID)+1)
	for id, a := range byID {
		s := timesheetProjectSummary{
			ProjectID: id,
			Name:      fmt.Sprintf("Project #%d", id),
			Unit:      "hours",
			Duration:  a.duration,
			Entries:   a.entries,
		}
		if p, ok := projectsByID[id]; ok {
			if name := strings.TrimSpace(p.Name); name != "" {
				s.Name = name
			}
			if u := strings.TrimSpace(p.Unit); u != "" {
				s.Unit = u
			}
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ProjectID < out[j].ProjectID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func generateTimesheetPDF(
	customerName, periodLabel string,
	days []timesheetDay,
	grandTotal time.Duration,
	projectSummaries []timesheetProjectSummary,
) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Timesheet", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(35, 7, "Customer:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 7, tr(customerName), "", "L", false)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(35, 7, "Period:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 7, tr(periodLabel), "", 1, "L", false, 0, "")
	pdf.Ln(6)

	const (
		colStart     = 16.0
		colStop      = 16.0
		colProject   = 40.0
		colComment   = 92.0
		colLength    = 16.0
		wrapLineHt   = 3.3
		paraGap      = 2.4
		noteSep      = 2.2
		cellPadY     = 1.5
		tableWidth   = colStart + colStop + colProject + colComment + colLength
		projectWidth = colProject - 2
		commentWidth = colComment - 2
		dayHeadingH  = 9.0
		tableHeaderH = 7.0
		dayTotalH    = 6.0
		dayGapH      = 4.0
		labelSpan    = colStart + colStop + colProject + colComment
	)

	// wrapPlain splits text already passed through the PDF encoding translator.
	// The current font must be the one used when the text is drawn.
	wrapPlain := func(text string, width float64) (lines []string, textH float64) {
		if text == "" {
			return []string{""}, wrapLineHt
		}
		wrapped := pdf.SplitLines([]byte(text), width)
		lines = make([]string, 0, len(wrapped))
		for _, w := range wrapped {
			lines = append(lines, string(w))
		}
		if len(lines) == 0 {
			lines = []string{""}
		}
		return lines, float64(len(lines)) * wrapLineHt
	}

	layoutComment := func(comment string) (paras [][]string, textH float64) {
		if comment == "" {
			return nil, wrapLineHt
		}
		raw := strings.ReplaceAll(comment, "\r\n", "\n")
		raw = strings.ReplaceAll(raw, "\r", "\n")
		parts := strings.Split(raw, "\n")
		for i, part := range parts {
			wrapped := pdf.SplitLines([]byte(part), commentWidth)
			lines := make([]string, 0, len(wrapped))
			for _, w := range wrapped {
				lines = append(lines, string(w))
			}
			if len(lines) == 0 {
				lines = []string{""}
			}
			if i > 0 {
				textH += paraGap
			}
			textH += float64(len(lines)) * wrapLineHt
			paras = append(paras, lines)
		}
		return paras, textH
	}

	type laidNote struct {
		projectLines []string
		commentParas [][]string
		length       string
		h            float64
	}

	layoutEntry := func(e timesheetDayEntry) (notes []laidNote, rowH float64) {
		src := e.Notes
		if len(src) == 0 {
			src = []timesheetNote{{}}
		}
		notes = make([]laidNote, len(src))
		for i, n := range src {
			pdf.SetFont("Arial", "B", 9)
			pLines, pH := wrapPlain(tr(strings.TrimSpace(n.Project)), projectWidth)
			pdf.SetFont("Arial", "", 9)
			paras, cH := layoutComment(tr(strings.TrimSpace(n.Comment)))
			h := pH
			if cH > h {
				h = cH
			}
			if wrapLineHt > h {
				h = wrapLineHt
			}
			notes[i] = laidNote{
				projectLines: pLines,
				commentParas: paras,
				length:       strings.TrimSpace(n.Length),
				h:            h,
			}
			rowH += h
		}
		if len(notes) > 1 {
			rowH += noteSep * float64(len(notes)-1)
		}
		rowH += 2 * cellPadY
		return notes, rowH
	}

	entryRowH := func(e timesheetDayEntry) float64 {
		_, rowH := layoutEntry(e)
		return rowH
	}

	dayContentH := func(day timesheetDay) float64 {
		h := dayHeadingH + tableHeaderH + dayTotalH
		for _, e := range day.Entries {
			h += entryRowH(e)
		}
		return h
	}

	pageBottomY := func() float64 {
		_, pageH := pdf.GetPageSize()
		_, _, _, bottom := pdf.GetMargins()
		return pageH - bottom
	}

	pageContentH := func() float64 {
		_, pageH := pdf.GetPageSize()
		_, top, _, bottom := pdf.GetMargins()
		return pageH - top - bottom
	}

	writeTableHeader := func() {
		pdf.SetFont("Arial", "B", 9)
		pdf.SetFillColor(230, 230, 230)
		pdf.CellFormat(colStart, tableHeaderH, "Start", "1", 0, "C", true, 0, "")
		pdf.CellFormat(colStop, tableHeaderH, "Stop", "1", 0, "C", true, 0, "")
		pdf.CellFormat(colProject, tableHeaderH, "Project", "1", 0, "L", true, 0, "")
		pdf.CellFormat(colComment, tableHeaderH, "Comment", "1", 0, "L", true, 0, "")
		pdf.CellFormat(colLength, tableHeaderH, "Length", "1", 1, "C", true, 0, "")
	}

	writeEntryRow := func(e timesheetDayEntry, stripe bool, allowPageBreak bool) {
		laid, rowH := layoutEntry(e)

		if allowPageBreak && pdf.GetY()+rowH > pageBottomY() {
			pdf.AddPage()
			writeTableHeader()
		}

		x0, y0 := pdf.GetX(), pdf.GetY()

		if stripe {
			pdf.SetFillColor(248, 248, 250)
			pdf.Rect(x0, y0, tableWidth, rowH, "F")
		}

		pdf.SetDrawColor(0, 0, 0)
		pdf.Rect(x0, y0, tableWidth, rowH, "D")
		xStop := x0 + colStart
		xProject := xStop + colStop
		xComment := xProject + colProject
		xLength := xComment + colComment
		pdf.Line(xStop, y0, xStop, y0+rowH)
		pdf.Line(xProject, y0, xProject, y0+rowH)
		pdf.Line(xComment, y0, xComment, y0+rowH)
		pdf.Line(xLength, y0, xLength, y0+rowH)

		pdf.SetFont("Arial", "", 9)
		topY := y0 + cellPadY
		pdf.SetXY(x0, topY)
		pdf.CellFormat(colStart, wrapLineHt, e.Start, "", 0, "C", false, 0, "")
		pdf.SetXY(xStop, topY)
		pdf.CellFormat(colStop, wrapLineHt, e.Stop, "", 0, "C", false, 0, "")

		y := y0 + cellPadY
		for i, n := range laid {
			if i > 0 {
				sepY := y + noteSep/2
				pdf.SetDrawColor(210, 210, 210)
				pdf.Line(xProject, sepY, x0+tableWidth, sepY)
				pdf.SetDrawColor(0, 0, 0)
				y += noteSep
			}
			pdf.SetFont("Arial", "B", 9)
			py := y
			for _, line := range n.projectLines {
				pdf.SetXY(xProject+1, py)
				pdf.CellFormat(projectWidth, wrapLineHt, line, "", 0, "L", false, 0, "")
				py += wrapLineHt
			}
			pdf.SetFont("Arial", "", 9)
			cy := y
			for pi, lines := range n.commentParas {
				if pi > 0 {
					cy += paraGap
				}
				for _, line := range lines {
					pdf.SetXY(xComment+1, cy)
					pdf.CellFormat(commentWidth, wrapLineHt, line, "", 0, "L", false, 0, "")
					cy += wrapLineHt
				}
			}
			if n.length != "" {
				pdf.SetXY(xLength, y)
				pdf.CellFormat(colLength, wrapLineHt, n.length, "", 0, "C", false, 0, "")
			}
			y += n.h
		}
		pdf.SetXY(x0, y0+rowH)
	}

	for i, day := range days {
		contentH := dayContentH(day)
		gap := 0.0
		if i > 0 {
			gap = dayGapH
		}

		need := gap + contentH
		if pdf.GetY()+need > pageBottomY() {
			_, top, _, _ := pdf.GetMargins()
			if pdf.GetY() > top+0.5 {
				pdf.AddPage()
			}
			gap = 0
		}

		allowSplit := contentH > pageContentH()

		if gap > 0 {
			pdf.Ln(gap)
		}

		pdf.SetFont("Arial", "B", 12)
		pdf.SetFillColor(245, 245, 245)
		pdf.SetDrawColor(180, 180, 180)
		pdf.CellFormat(tableWidth, dayHeadingH, formatDayHeading(day.Date), "", 1, "L", true, 0, "")
		x, y := pdf.GetX(), pdf.GetY()
		pdf.Line(x, y, x+tableWidth, y)
		pdf.SetDrawColor(0, 0, 0)

		writeTableHeader()

		for j, e := range day.Entries {
			writeEntryRow(e, j%2 == 1, allowSplit)
		}

		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(labelSpan, dayTotalH, "Day total", "", 0, "R", false, 0, "")
		pdf.CellFormat(colLength, dayTotalH, formatHM(day.Total), "", 1, "C", false, 0, "")
	}

	pdf.Ln(4)
	if pdf.GetY() > 270 {
		pdf.AddPage()
	}
	pdf.SetFont("Arial", "B", 12)
	pdf.SetFillColor(220, 235, 250)
	pdf.CellFormat(labelSpan, 9, "Total", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colLength, 9, formatHM(grandTotal), "1", 1, "C", true, 0, "")

	writeProjectSummaryTable(pdf, tr, projectSummaries, grandTotal, tableWidth)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeProjectSummaryTable renders the invoice-oriented breakdown at the end of the PDF:
// Project | Amount | Unit (unit is a free-text label; amount is reported duration).
func writeProjectSummaryTable(
	pdf *fpdf.Fpdf,
	tr func(string) string,
	summaries []timesheetProjectSummary,
	grandTotal time.Duration,
	tableWidth float64,
) {
	if len(summaries) == 0 {
		return
	}

	const (
		colProject = 100.0
		colAmount  = 40.0
		colUnit    = 40.0
		rowH       = 7.0
		headerH    = 8.0
	)

	needH := 8.0 + headerH + float64(len(summaries)+1)*rowH + 10.0
	_, pageH := pdf.GetPageSize()
	_, _, _, bottom := pdf.GetMargins()
	if pdf.GetY()+needH > pageH-bottom {
		pdf.AddPage()
	}

	pdf.Ln(8)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Summary by project", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(80, 80, 80)
	// pdf.CellFormat(0, 5, "For invoicing amount is reported time; unit is set on each project", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(2)

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(colProject, headerH, "Project", "1", 0, "L", true, 0, "")
	pdf.CellFormat(colAmount, headerH, "Amount", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colUnit, headerH, "Unit", "1", 1, "C", true, 0, "")

	for i, s := range summaries {
		if pdf.GetY()+rowH > pageH-bottom {
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 9)
			pdf.SetFillColor(230, 230, 230)
			pdf.CellFormat(colProject, headerH, "Project", "1", 0, "L", true, 0, "")
			pdf.CellFormat(colAmount, headerH, "Amount", "1", 0, "C", true, 0, "")
			pdf.CellFormat(colUnit, headerH, "Unit", "1", 1, "C", true, 0, "")
		}

		if i%2 == 1 {
			pdf.SetFillColor(248, 248, 250)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		pdf.SetFont("Arial", "", 9)
		pdf.CellFormat(colProject, rowH, tr(s.Name), "1", 0, "L", true, 0, "")
		pdf.CellFormat(colAmount, rowH, formatHM(s.Duration), "1", 0, "C", true, 0, "")
		pdf.CellFormat(colUnit, rowH, tr(s.Unit), "1", 1, "C", true, 0, "")
	}

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(220, 235, 250)
	pdf.CellFormat(colProject, rowH, "Total (reported time)", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colAmount, rowH, formatHM(grandTotal), "1", 0, "C", true, 0, "")
	pdf.CellFormat(colUnit, rowH, "", "1", 1, "C", true, 0, "")
	_ = tableWidth
}

// parseHMDuration parses "H:MM", "HH:MM", or "H:MM:SS" into a duration.
func parseHMDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid duration")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 {
		return 0, fmt.Errorf("invalid hours")
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid minutes")
	}
	sec := 0
	if len(parts) == 3 {
		sec, err = strconv.Atoi(parts[2])
		if err != nil || sec < 0 || sec > 59 {
			return 0, fmt.Errorf("invalid seconds")
		}
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
}

func formatDayHeading(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("2006-01-02 Monday")
}

// monthRange is the UTC instant window for a calendar month in the viewer's
// timezone. The stored instants stay UTC; only the window edges move.
func (h *TimesheetsHandler) monthRange(c echo.Context, monthStr string) (string, time.Time, time.Time, *time.Location, error) {
	loc, err := viewerLocation(h.DB, c)
	if err != nil {
		return "", time.Time{}, time.Time{}, nil, err
	}
	label, start, end, err := parseMonthParam(monthStr, loc)
	return label, start, end, loc, err
}

// dayWindowInMonth is one calendar day inside monthStr, both in loc.
// dateStr is YYYY-MM-DD. The window is [midnight, next midnight) in that zone.
func dayWindowInMonth(dateStr, monthStr string, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("date is required")
	}
	dayStart, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("date must be YYYY-MM-DD")
	}
	if dayStart.Format("2006-01") != monthStr {
		return time.Time{}, time.Time{}, fmt.Errorf("date must fall in the given month")
	}
	return dayStart, dayStart.AddDate(0, 0, 1), nil
}

func parseMonthParam(monthStr string, loc *time.Location) (string, time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	if monthStr == "" {
		monthStr = time.Now().In(loc).Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", monthStr, loc)
	if err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("month must be YYYY-MM")
	}
	end := start.AddDate(0, 1, 0)
	return monthStr, start, end, nil
}

// viewerLocation is the timezone used to present dates. The query parameter
// wins (the browser sends it). Otherwise the profile timezone is used. An
// empty profile means UTC for comparisons; the web UI sends the browser zone
// instead of leaving that empty.
func viewerLocation(db *gorm.DB, c echo.Context) (*time.Location, error) {
	tz := strings.TrimSpace(c.QueryParam("timezone"))
	if tz == "" && db != nil {
		uid := middleware.UserIDFromContext(c)
		if uid > 0 {
			var profile string
			_ = db.Model(&models.User{}).Where("_id = ?", uid).Pluck("timezone", &profile).Error
			tz = strings.TrimSpace(profile)
		}
	}
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("timezone must be an IANA name such as Europe/Stockholm")
	}
	return loc, nil
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "customer"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "customer"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return strings.ToLower(out)
}
