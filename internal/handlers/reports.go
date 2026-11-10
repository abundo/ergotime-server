package handlers

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abundo/ergotime-server/internal/database"
	"github.com/abundo/ergotime-server/internal/dto"
	"github.com/abundo/ergotime-server/internal/middleware"
	"github.com/abundo/ergotime-server/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ReportsHandler manages time reports and client sync.
type ReportsHandler struct {
	DB *gorm.DB
}

// scopeReportsForUser forces non-admins to only see their own reports.
// Admins may filter by user_id query param; if omitted they see all.
func scopeReportsForUser(c echo.Context, q *gorm.DB) *gorm.DB {
	selfID := middleware.UserIDFromContext(c)
	isAdmin := middleware.IsAdminFromContext(c)

	if uid := c.QueryParam("user_id"); uid != "" {
		id, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			return q.Where("1 = 0") // invalid → empty
		}
		if !isAdmin && id != selfID {
			return q.Where("1 = 0")
		}
		return q.Where("user_id = ?", id)
	}
	if !isAdmin {
		return q.Where("user_id = ?", selfID)
	}
	return q
}

func (h *ReportsHandler) List(c echo.Context) error {
	q := h.DB.Model(&models.Report{}).Order("start DESC")
	q = scopeReportsForUser(c, q)

	if pid := c.QueryParam("project_id"); pid != "" {
		id, err := strconv.ParseInt(pid, 10, 64)
		if err != nil {
			return badRequest(c, "invalid project_id")
		}
		q = q.Where("project_id = ? OR _id IN (SELECT report_id FROM report_line WHERE project_id = ?)", id, id)
	}
	if cid := c.QueryParam("customer_id"); cid != "" {
		id, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			return badRequest(c, "invalid customer_id")
		}
		q = q.Where(
			"customer_id = ? OR project_id IN (?)",
			id,
			h.DB.Model(&models.Project{}).Select("_id").Where("customer_id = ?", id),
		)
	}
	if c.QueryParam("include_deleted") != "1" && c.QueryParam("include_deleted") != "true" {
		q = q.Where("deleted = ?", false)
	}
	if from := c.QueryParam("from"); from != "" {
		t, err := dto.ParseTime(from)
		if err != nil {
			return badRequest(c, "invalid from datetime")
		}
		q = q.Where("start >= ?", t)
	}
	if to := c.QueryParam("to"); to != "" {
		t, err := dto.ParseTime(to)
		if err != nil {
			return badRequest(c, "invalid to datetime")
		}
		q = q.Where("start <= ?", t)
	}
	if limit := c.QueryParam("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 10000 {
			return badRequest(c, "invalid limit")
		}
		q = q.Limit(n)
	}
	if offset := c.QueryParam("offset"); offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 {
			return badRequest(c, "invalid offset")
		}
		q = q.Offset(n)
	}

	var items []models.Report
	if err := q.Find(&items).Error; err != nil {
		return internal(c, "failed to list reports")
	}
	if err := attachLines(h.DB, items); err != nil {
		return internal(c, "failed to load report lines")
	}

	if acceptsLegacy(c) {
		out := make([]dto.LegacyReport, 0, len(items))
		for i := range items {
			out = append(out, dto.LegacyReportFrom(&items[i]))
		}
		return okData(c, out)
	}
	return okData(c, items)
}

func (h *ReportsHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var item models.Report
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "report not found")
		}
		return internal(c, "database error")
	}
	if !canAccessReport(c, &item) {
		return notFound(c, "report not found")
	}
	loaded := []models.Report{item}
	if err := attachLines(h.DB, loaded); err != nil {
		return internal(c, "failed to load report lines")
	}
	item = loaded[0]
	if acceptsLegacy(c) {
		return okData(c, dto.LegacyReportFrom(&item))
	}
	return ok(c, item)
}

// Sync returns reports with seq > :seq for desktop client incremental sync.
// Always scoped to the authenticated user (desktop is multiuser per account).
func (h *ReportsHandler) Sync(c echo.Context) error {
	seq, err := strconv.ParseInt(c.Param("seq"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid seq")
	}

	selfID := middleware.UserIDFromContext(c)
	q := h.DB.Model(&models.Report{}).
		Where("seq > ? AND user_id = ?", seq, selfID).
		Order("seq ASC")

	if maxage := c.QueryParam("maxage"); maxage != "" {
		days, err := strconv.Atoi(maxage)
		if err != nil || days < 0 {
			return badRequest(c, "invalid maxage")
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		q = q.Where("modified > ?", cutoff)
	}
	if limit := c.QueryParam("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 {
			return badRequest(c, "invalid limit")
		}
		q = q.Limit(n)
	}
	if offset := c.QueryParam("offset"); offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 {
			return badRequest(c, "invalid offset")
		}
		q = q.Offset(n)
	}

	var items []models.Report
	if err := q.Find(&items).Error; err != nil {
		return internal(c, "failed to sync reports")
	}
	if err := attachLines(h.DB, items); err != nil {
		return internal(c, "failed to load report lines")
	}

	out := make([]dto.LegacyReport, 0, len(items))
	for i := range items {
		out = append(out, dto.LegacyReportFrom(&items[i]))
	}
	return okData(c, out)
}

func (h *ReportsHandler) Create(c echo.Context) error {
	item, err := h.bindReport(c, nil)
	if err != nil {
		return err
	}

	selfID := middleware.UserIDFromContext(c)
	isAdmin := middleware.IsAdminFromContext(c)

	// Non-admins always own their reports; admins may set user_id.
	if item.UserID == 0 || !isAdmin {
		item.UserID = selfID
	}
	if item.Start.IsZero() {
		return badRequest(c, "start is required")
	}

	// Idempotent create: if client_uuid already exists for this user, return it.
	if item.ClientUUID != nil && *item.ClientUUID != "" {
		var existing models.Report
		err := h.DB.Where("client_uuid = ? AND user_id = ?", *item.ClientUUID, item.UserID).
			First(&existing).Error
		if err == nil {
			loaded := []models.Report{existing}
			_ = attachLines(h.DB, loaded)
			existing = loaded[0]
			return h.respondReport(c, &existing, false)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return internal(c, "database error")
		}
	}

	seq, err := database.NextReportSeq(h.DB)
	if err != nil {
		return internal(c, "failed to allocate seq")
	}
	item.Seq = seq
	item.Modified = time.Now().UTC()

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		return writeReport(tx, item, true)
	})
	if err != nil {
		if msg, ok := lineError(err); ok {
			return badRequest(c, msg)
		}
		if item.ClientUUID != nil && *item.ClientUUID != "" {
			var existing models.Report
			if err2 := h.DB.Where("client_uuid = ? AND user_id = ?", *item.ClientUUID, item.UserID).
				First(&existing).Error; err2 == nil {
				loaded := []models.Report{existing}
				_ = attachLines(h.DB, loaded)
				existing = loaded[0]
				return h.respondReport(c, &existing, false)
			}
		}
		return internal(c, "failed to create report")
	}

	return h.respondReport(c, item, true)
}

func (h *ReportsHandler) respondReport(c echo.Context, item *models.Report, isNew bool) error {
	if acceptsLegacy(c) || c.Request().Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		return ok(c, map[string]int64{"_id": item.ID})
	}
	if isNew {
		return created(c, item)
	}
	return ok(c, item)
}

func (h *ReportsHandler) Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var existing models.Report
	if err := h.DB.First(&existing, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "report not found")
		}
		return internal(c, "database error")
	}
	if !canAccessReport(c, &existing) {
		return notFound(c, "report not found")
	}

	updated, err := h.bindReport(c, &existing)
	if err != nil {
		return err
	}

	// Non-admins cannot reassign ownership
	if !middleware.IsAdminFromContext(c) {
		updated.UserID = existing.UserID
	}
	seq, err := database.NextReportSeq(h.DB)
	if err != nil {
		return internal(c, "failed to allocate seq")
	}
	updated.Seq = seq
	updated.Modified = time.Now().UTC()
	updated.ID = id

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		return writeReport(tx, updated, false)
	})
	if err != nil {
		if msg, ok := lineError(err); ok {
			return badRequest(c, msg)
		}
		return internal(c, "failed to update report")
	}

	if acceptsLegacy(c) || c.Request().Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		return ok(c, map[string]int64{"id": updated.ID})
	}
	return ok(c, updated)
}

// Delete soft-deletes a report (sets deleted=true) and bumps seq for sync.
func (h *ReportsHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var item models.Report
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "report not found")
		}
		return internal(c, "database error")
	}
	if !canAccessReport(c, &item) {
		return notFound(c, "report not found")
	}

	seq, err := database.NextReportSeq(h.DB)
	if err != nil {
		return internal(c, "failed to allocate seq")
	}
	item.Deleted = true
	item.Seq = seq
	item.Modified = time.Now().UTC()

	if err := h.DB.Save(&item).Error; err != nil {
		return internal(c, "failed to delete report")
	}
	return ok(c, item)
}

// BatchPushRequest is the body for POST /api/reports/batch (desktop client).
// Ops are applied in order: deletes, then creates, then updates.
type BatchPushRequest struct {
	Deletes []int64           `json:"deletes"`
	Creates []BatchCreateItem `json:"creates"`
	Updates []BatchUpdateItem `json:"updates"`
}

// BatchCreateItem is one new report in a batch push.
type BatchCreateItem struct {
	ClientUUID string              `json:"client_uuid"`
	ProjectID  int64               `json:"project_id"`
	Start      string              `json:"start"`
	Stop       string              `json:"stop"`
	Comment    string              `json:"comment"`
	Lines      []models.ReportLine `json:"lines"`
	UserID     int64               `json:"user_id"`
}

// BatchUpdateItem is one existing report update in a batch push.
type BatchUpdateItem struct {
	ID         int64               `json:"id"`
	ProjectID  int64               `json:"project_id"`
	Start      string              `json:"start"`
	Stop       string              `json:"stop"`
	Comment    string              `json:"comment"`
	Lines      []models.ReportLine `json:"lines"`
	ClientUUID string              `json:"client_uuid"`
}

// BatchPushResponse returns server state after each op.
type BatchPushResponse struct {
	Deleted []BatchDeletedItem `json:"deleted"`
	Created []models.Report    `json:"created"`
	Updated []models.Report    `json:"updated"`
}

// BatchDeletedItem confirms a soft-delete.
type BatchDeletedItem struct {
	ID  int64 `json:"id"`
	Seq int64 `json:"seq"`
}

// Batch applies deletes, creates, and updates in one request.
// Create is idempotent on client_uuid. Failed ops abort the rest of that
// section and return HTTP 400/500 with a message; successful earlier ops
// in the same request are kept (client re-runs remaining via dirty flags).
func (h *ReportsHandler) Batch(c echo.Context) error {
	var req BatchPushRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}

	selfID := middleware.UserIDFromContext(c)
	isAdmin := middleware.IsAdminFromContext(c)
	out := BatchPushResponse{
		Deleted: make([]BatchDeletedItem, 0, len(req.Deletes)),
		Created: make([]models.Report, 0, len(req.Creates)),
		Updated: make([]models.Report, 0, len(req.Updates)),
	}

	for _, id := range req.Deletes {
		if id <= 0 {
			continue
		}
		var item models.Report
		if err := h.DB.First(&item, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Already gone — treat as success for idempotency.
				out.Deleted = append(out.Deleted, BatchDeletedItem{ID: id, Seq: 0})
				continue
			}
			return internal(c, "database error")
		}
		if !canAccessReport(c, &item) {
			return notFound(c, fmt.Sprintf("report %d not found", id))
		}
		if item.Deleted {
			out.Deleted = append(out.Deleted, BatchDeletedItem{ID: item.ID, Seq: item.Seq})
			continue
		}
		seq, err := database.NextReportSeq(h.DB)
		if err != nil {
			return internal(c, "failed to allocate seq")
		}
		item.Deleted = true
		item.Seq = seq
		item.Modified = time.Now().UTC()
		if err := h.DB.Save(&item).Error; err != nil {
			return internal(c, "failed to delete report")
		}
		out.Deleted = append(out.Deleted, BatchDeletedItem{ID: item.ID, Seq: item.Seq})
	}

	for _, in := range req.Creates {
		item := &models.Report{
			ProjectID: in.ProjectID,
			Comment:   in.Comment,
			UserID:    in.UserID,
			Lines:     in.Lines,
			LinesSet:  in.Lines != nil,
		}
		if item.UserID == 0 || !isAdmin {
			item.UserID = selfID
		}
		start, err := dto.ParseTime(in.Start)
		if err != nil || start.IsZero() {
			return badRequest(c, "create: invalid start time")
		}
		item.Start = start
		if strings.TrimSpace(in.Stop) != "" {
			stop, err := dto.ParseTime(in.Stop)
			if err != nil {
				return badRequest(c, "create: invalid stop time")
			}
			item.Stop = &stop
		}
		if cu := strings.TrimSpace(in.ClientUUID); cu != "" {
			item.ClientUUID = &cu
			var existing models.Report
			err := h.DB.Where("client_uuid = ? AND user_id = ?", cu, item.UserID).First(&existing).Error
			if err == nil {
				loaded := []models.Report{existing}
				_ = attachLines(h.DB, loaded)
				out.Created = append(out.Created, loaded[0])
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return internal(c, "database error")
			}
		}
		seq, err := database.NextReportSeq(h.DB)
		if err != nil {
			return internal(c, "failed to allocate seq")
		}
		item.Seq = seq
		item.Modified = time.Now().UTC()
		err = h.DB.Transaction(func(tx *gorm.DB) error {
			return writeReport(tx, item, true)
		})
		if err != nil {
			if msg, ok := lineError(err); ok {
				return badRequest(c, "create: "+msg)
			}
			if item.ClientUUID != nil {
				var existing models.Report
				if err2 := h.DB.Where("client_uuid = ? AND user_id = ?", *item.ClientUUID, item.UserID).
					First(&existing).Error; err2 == nil {
					loaded := []models.Report{existing}
					_ = attachLines(h.DB, loaded)
					out.Created = append(out.Created, loaded[0])
					continue
				}
			}
			return internal(c, "failed to create report")
		}
		out.Created = append(out.Created, *item)
	}

	for _, in := range req.Updates {
		if in.ID <= 0 {
			return badRequest(c, "update: id is required")
		}
		var existing models.Report
		if err := h.DB.First(&existing, in.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound(c, fmt.Sprintf("report %d not found", in.ID))
			}
			return internal(c, "database error")
		}
		if !canAccessReport(c, &existing) {
			return notFound(c, fmt.Sprintf("report %d not found", in.ID))
		}
		if in.ProjectID > 0 {
			existing.ProjectID = in.ProjectID
		}
		if in.Lines != nil {
			existing.Lines = in.Lines
			existing.LinesSet = true
		}
		if in.Start != "" {
			t, err := dto.ParseTime(in.Start)
			if err != nil {
				return badRequest(c, "update: invalid start time")
			}
			existing.Start = t
		}
		if in.Stop == "" {
			existing.Stop = nil
		} else {
			t, err := dto.ParseTime(in.Stop)
			if err != nil {
				return badRequest(c, "update: invalid stop time")
			}
			existing.Stop = &t
		}
		existing.Comment = in.Comment
		if cu := strings.TrimSpace(in.ClientUUID); cu != "" {
			existing.ClientUUID = &cu
		}
		if !isAdmin {
			// ownership stays
		}
		seq, err := database.NextReportSeq(h.DB)
		if err != nil {
			return internal(c, "failed to allocate seq")
		}
		existing.Seq = seq
		existing.Modified = time.Now().UTC()
		existing.Deleted = false
		err = h.DB.Transaction(func(tx *gorm.DB) error {
			return writeReport(tx, &existing, false)
		})
		if err != nil {
			if msg, ok := lineError(err); ok {
				return badRequest(c, "update: "+msg)
			}
			return internal(c, "failed to update report")
		}
		out.Updated = append(out.Updated, existing)
	}

	return ok(c, out)
}

// Monthly returns aggregated monthly report data for the SPA.
func (h *ReportsHandler) Monthly(c echo.Context) error {
	selfID := middleware.UserIDFromContext(c)
	isAdmin := middleware.IsAdminFromContext(c)

	userID, _ := strconv.ParseInt(c.QueryParam("user_id"), 10, 64)
	if userID == 0 {
		userID = selfID
	} else if !isAdmin && userID != selfID {
		return forbidden(c, "cannot view other users' reports")
	}

	customerID, _ := strconv.ParseInt(c.QueryParam("customer_id"), 10, 64)
	projectID, _ := strconv.ParseInt(c.QueryParam("project_id"), 10, 64)
	loc, err := viewerLocation(h.DB, c)
	if err != nil {
		return badRequest(c, err.Error())
	}
	monthStr, start, end, err := parseMonthParam(c.QueryParam("month"), loc)
	if err != nil {
		return badRequest(c, err.Error())
	}

	q := h.DB.Where(
		"user_id = ? AND deleted = ? AND stop IS NOT NULL AND start >= ? AND start < ?",
		userID, false, start, end,
	)
	if customerID > 0 {
		q = q.Where(
			`(customer_id = ? OR project_id IN (SELECT _id FROM project WHERE customer_id = ?))`,
			customerID, customerID,
		)
	}
	if projectID > 0 {
		q = q.Where(
			`(project_id = ? OR _id IN (SELECT report_id FROM report_line WHERE project_id = ?))`,
			projectID, projectID,
		)
	}
	var reports []models.Report
	if err := q.Order("start ASC").Find(&reports).Error; err != nil {
		return internal(c, "failed to load reports")
	}
	if err := attachLines(h.DB, reports); err != nil {
		return internal(c, "failed to load report lines")
	}

	projectsByID, err := loadReportProjects(h.DB, reports)
	if err != nil {
		return internal(c, "failed to load projects")
	}
	customersByID, err := loadReportCustomers(h.DB, reports, projectsByID)
	if err != nil {
		return internal(c, "failed to load customers")
	}

	catalog, err := loadCustomerProjects(h.DB, customerIDsInReports(reports, projectsByID, customerID))
	if err != nil {
		return internal(c, "failed to load projects")
	}

	customers, grandTotal := buildMonthlyCustomers(reports, projectsByID, customersByID, catalog, customerID, projectID, loc)
	resp := dto.MonthlyReportResponse{
		Month:      monthStr,
		UserID:     userID,
		CustomerID: customerID,
		ProjectID:  projectID,
		TotalHours: grandTotal.Hours(),
		TotalHM:    formatHM(grandTotal),
		Customers:  customers,
	}
	return ok(c, resp)
}

func loadReportProjects(db *gorm.DB, reports []models.Report) (map[int64]models.Project, error) {
	ids := map[int64]struct{}{}
	for _, r := range reports {
		if r.ProjectID > 0 {
			ids[r.ProjectID] = struct{}{}
		}
		for _, ln := range r.Lines {
			if ln.ProjectID > 0 {
				ids[ln.ProjectID] = struct{}{}
			}
		}
	}
	out := map[int64]models.Project{}
	if len(ids) == 0 {
		return out, nil
	}
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	var projects []models.Project
	if err := db.Where("_id IN ?", list).Find(&projects).Error; err != nil {
		return nil, err
	}
	for _, p := range projects {
		out[p.ID] = p
	}
	return out, nil
}

func customerIDsInReports(reports []models.Report, projectsByID map[int64]models.Project, filterCustomerID int64) []int64 {
	if filterCustomerID > 0 {
		return []int64{filterCustomerID}
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, r := range reports {
		id := reportCustomerID(r, projectsByID)
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// loadCustomerProjects loads every project of the customers in a monthly report,
// including active projects that have no time in the month.
func loadCustomerProjects(db *gorm.DB, customerIDs []int64) ([]models.Project, error) {
	if len(customerIDs) == 0 {
		return nil, nil
	}
	var projects []models.Project
	if err := db.Where("customer_id IN ?", customerIDs).Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func loadReportCustomers(db *gorm.DB, reports []models.Report, projectsByID map[int64]models.Project) (map[int64]models.Customer, error) {
	ids := map[int64]struct{}{}
	for _, r := range reports {
		if id := reportCustomerID(r, projectsByID); id > 0 {
			ids[id] = struct{}{}
		}
	}
	out := map[int64]models.Customer{}
	if len(ids) == 0 {
		return out, nil
	}
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	var customers []models.Customer
	if err := db.Where("_id IN ?", list).Find(&customers).Error; err != nil {
		return nil, err
	}
	for _, cu := range customers {
		out[cu.ID] = cu
	}
	return out, nil
}

func reportCustomerID(r models.Report, projectsByID map[int64]models.Project) int64 {
	if r.CustomerID > 0 {
		return r.CustomerID
	}
	pid, shared := models.AttributedProject(r)
	if !shared {
		if p, ok := projectsByID[pid]; ok && p.CustomerID > 0 {
			return p.CustomerID
		}
	}
	if len(r.Lines) > 0 {
		if p, ok := projectsByID[r.Lines[0].ProjectID]; ok && p.CustomerID > 0 {
			return p.CustomerID
		}
	}
	if p, ok := projectsByID[r.ProjectID]; ok {
		return p.CustomerID
	}
	return 0
}

// buildMonthlyCustomers groups a month the same way as the timesheet PDF:
// one customer, then each day, and on each interval the project followed by
// that project's comment. Several projects stay on the one interval.
// filterCustomerID drops other customers when the request named one.
// catalog is every project of those customers. The summary lists each active
// one, including projects with no time in the month.
// filterProjectID limits that summary to one project when the request named one.
func buildMonthlyCustomers(
	reports []models.Report,
	projectsByID map[int64]models.Project,
	customersByID map[int64]models.Customer,
	catalog []models.Project,
	filterCustomerID int64,
	filterProjectID int64,
	loc *time.Location,
) ([]dto.MonthlyCustomer, time.Duration) {
	byCust := map[int64][]models.Report{}
	var custIDs []int64
	seen := map[int64]bool{}
	for _, r := range reports {
		cid := reportCustomerID(r, projectsByID)
		if filterCustomerID > 0 && cid != filterCustomerID {
			continue
		}
		if !seen[cid] {
			seen[cid] = true
			custIDs = append(custIDs, cid)
		}
		byCust[cid] = append(byCust[cid], r)
	}

	sort.Slice(custIDs, func(i, j int) bool {
		ni, nj := customerSortName(custIDs[i], customersByID), customerSortName(custIDs[j], customersByID)
		if ni == nj {
			return custIDs[i] < custIDs[j]
		}
		return ni < nj
	})

	names := map[int64]string{}
	for id, p := range projectsByID {
		names[id] = p.Name
	}

	out := make([]dto.MonthlyCustomer, 0, len(custIDs))
	var grand time.Duration
	for _, cid := range custIDs {
		reps := byCust[cid]
		days, total := groupReportsByDay(reps, names, loc)
		if len(days) == 0 {
			continue
		}
		cu := customersByID[cid]
		mc := dto.MonthlyCustomer{
			CustomerID:  cid,
			Name:        strings.TrimSpace(cu.Name),
			Description: cu.Description,
			TotalHours:  total.Hours(),
			TotalHM:     formatHM(total),
			Invoiced:    allReportsInvoiced(reps),
			Days:        monthlyDays(days),
			Projects:    monthlyProjectSummaries(reps, projectsByID, projectsForCustomer(catalog, cid, filterProjectID)),
		}
		if mc.Name == "" {
			if cid > 0 {
				mc.Name = fmt.Sprintf("Customer #%d", cid)
			} else {
				mc.Name = "Unknown customer"
			}
		}
		grand += total
		out = append(out, mc)
	}
	return out, grand
}

func customerSortName(id int64, customersByID map[int64]models.Customer) string {
	if cu, ok := customersByID[id]; ok {
		if name := strings.ToLower(strings.TrimSpace(cu.Name)); name != "" {
			return name
		}
	}
	return fmt.Sprintf("customer #%d", id)
}

func monthlyDays(days []timesheetDay) []dto.MonthlyDay {
	out := make([]dto.MonthlyDay, 0, len(days))
	for _, day := range days {
		md := dto.MonthlyDay{
			Date:       day.Date,
			TotalHours: day.Total.Hours(),
			TotalHM:    formatHM(day.Total),
			Entries:    make([]dto.MonthlyEntry, 0, len(day.Entries)),
		}
		for _, e := range day.Entries {
			notes := make([]dto.MonthlyNote, 0, len(e.Notes))
			for _, n := range e.Notes {
				notes = append(notes, dto.MonthlyNote{Project: n.Project, Comment: n.Comment})
			}
			if len(notes) == 0 {
				notes = []dto.MonthlyNote{{}}
			}
			md.Entries = append(md.Entries, dto.MonthlyEntry{
				ID:    e.ID,
				Start: e.Start,
				Stop:  e.Stop,
				Hours: e.Duration.Hours(),
				HM:    e.Length,
				Notes: notes,
			})
		}
		out = append(out, md)
	}
	return out
}

func projectsForCustomer(catalog []models.Project, customerID, filterProjectID int64) []models.Project {
	out := make([]models.Project, 0, len(catalog))
	for _, p := range catalog {
		if p.CustomerID != customerID {
			continue
		}
		if filterProjectID > 0 && p.ID != filterProjectID {
			continue
		}
		out = append(out, p)
	}
	return out
}

func allReportsInvoiced(reports []models.Report) bool {
	if len(reports) == 0 {
		return false
	}
	for _, r := range reports {
		if !r.Invoiced {
			return false
		}
	}
	return true
}

// projectSummariesForCustomer is the summary table shared by the monthly report
// and the timesheet PDF. Time is counted on a project only when the interval
// names that project alone. Shared intervals are left off. Every active project
// is listed, with zero time when it has none. An inactive project is listed
// only when it has time.
func projectSummariesForCustomer(reports []models.Report, projectsByID map[int64]models.Project, catalog []models.Project) []timesheetProjectSummary {
	sums := summarizeReportsByProject(reports, projectsByID)
	out := make([]timesheetProjectSummary, 0, len(sums)+len(catalog))
	seen := map[int64]bool{}
	for _, s := range sums {
		if s.ProjectID == 0 {
			continue
		}
		out = append(out, s)
		seen[s.ProjectID] = true
	}
	for _, p := range catalog {
		if !p.Active || seen[p.ID] {
			continue
		}
		unit := strings.TrimSpace(p.Unit)
		if unit == "" {
			unit = "hours"
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = fmt.Sprintf("Project #%d", p.ID)
		}
		out = append(out, timesheetProjectSummary{
			ProjectID: p.ID,
			Name:      name,
			Unit:      unit,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni == nj {
			return out[i].ProjectID < out[j].ProjectID
		}
		return ni < nj
	})
	return out
}

// monthlyProjectSummaries is the customer's "Summary by project".
func monthlyProjectSummaries(reports []models.Report, projectsByID map[int64]models.Project, catalog []models.Project) []dto.MonthlyProject {
	lookup := make(map[int64]models.Project, len(projectsByID)+len(catalog))
	for id, p := range projectsByID {
		lookup[id] = p
	}
	for _, p := range catalog {
		lookup[p.ID] = p
	}
	sums := projectSummariesForCustomer(reports, projectsByID, catalog)
	out := make([]dto.MonthlyProject, 0, len(sums))
	for _, s := range sums {
		row := dto.MonthlyProject{
			ProjectID:  s.ProjectID,
			Name:       s.Name,
			Unit:       s.Unit,
			TotalHours: s.Duration.Hours(),
			TotalHM:    formatHM(s.Duration),
		}
		if p, ok := lookup[s.ProjectID]; ok {
			row.CostCenter = p.CostCenter
		}
		out = append(out, row)
	}
	return out
}

func formatHM(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalMin := int(d.Minutes())
	h := totalMin / 60
	m := totalMin % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func canAccessReport(c echo.Context, r *models.Report) bool {
	if middleware.IsAdminFromContext(c) {
		return true
	}
	return r.UserID == middleware.UserIDFromContext(c)
}

func (h *ReportsHandler) projectExists(projectID int64) bool {
	var n int64
	if err := h.DB.Model(&models.Project{}).Where("_id = ?", projectID).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// bindReport parses JSON or form body into a report model.
func (h *ReportsHandler) bindReport(c echo.Context, base *models.Report) (*models.Report, error) {
	item := &models.Report{}
	if base != nil {
		*item = *base
	}

	ct := c.Request().Header.Get(echo.HeaderContentType)
	if strings.Contains(ct, "application/json") {
		var raw map[string]any
		if err := c.Bind(&raw); err != nil {
			return nil, badRequest(c, "invalid request body")
		}

		if v, ok := asInt64(raw["user_id"]); ok {
			item.UserID = v
		}
		if v, ok := asInt64(raw["project_id"]); ok {
			item.ProjectID = v
		}
		if s, ok := raw["start"].(string); ok && s != "" {
			t, err := dto.ParseTime(s)
			if err != nil {
				return nil, badRequest(c, "invalid start time")
			}
			item.Start = t
		}
		if s, ok := raw["stop"].(string); ok {
			if s == "" {
				item.Stop = nil
			} else {
				t, err := dto.ParseTime(s)
				if err != nil {
					return nil, badRequest(c, "invalid stop time")
				}
				item.Stop = &t
			}
		}
		if s, ok := raw["comment"].(string); ok {
			item.Comment = s
		} else if s, ok := raw["commment"].(string); ok { // legacy typo
			item.Comment = s
		}
		if rawLines, ok := raw["lines"].([]any); ok {
			item.LinesSet = true
			item.Lines = nil
			for _, rawLn := range rawLines {
				m, ok := rawLn.(map[string]any)
				if !ok {
					continue
				}
				pid, _ := asInt64(m["project_id"])
				comment, _ := m["comment"].(string)
				item.Lines = append(item.Lines, models.ReportLine{
					ProjectID: pid,
					Comment:   comment,
				})
			}
		}
		if v, ok := asBool(raw["deleted"]); ok {
			item.Deleted = v
		}
		if s, ok := raw["client_uuid"].(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				item.ClientUUID = &s
			}
		}
		return item, nil
	}

	// form-urlencoded
	if err := c.Request().ParseForm(); err != nil {
		return nil, badRequest(c, "invalid form body")
	}
	form := c.Request().PostForm
	if v := form.Get("user_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, badRequest(c, "invalid user_id")
		}
		item.UserID = id
	}
	if v := form.Get("project_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, badRequest(c, "invalid project_id")
		}
		item.ProjectID = id
	}
	if v := form.Get("start"); v != "" {
		t, err := dto.ParseTime(v)
		if err != nil {
			return nil, badRequest(c, "invalid start time")
		}
		item.Start = t
	}
	if v := form.Get("stop"); v != "" {
		t, err := dto.ParseTime(v)
		if err != nil {
			return nil, badRequest(c, "invalid stop time")
		}
		item.Stop = &t
	}
	if v, ok := form["comment"]; ok {
		item.Comment = v[0]
	} else if v, ok := form["commment"]; ok {
		item.Comment = v[0]
	}
	if v := form.Get("deleted"); v != "" {
		item.Deleted = v == "1" || strings.EqualFold(v, "true")
	}
	if v := strings.TrimSpace(form.Get("client_uuid")); v != "" {
		item.ClientUUID = &v
	}
	if v := form.Get("_id"); v != "" && item.ID == 0 {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			item.ID = id
		}
	}
	return item, nil
}

func asInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func asBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case float64:
		return t != 0, true
	case string:
		return t == "1" || strings.EqualFold(t, "true"), true
	default:
		return false, false
	}
}
