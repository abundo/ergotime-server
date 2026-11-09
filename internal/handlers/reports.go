package handlers

import (
	"errors"
	"fmt"
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
		q = q.Where("project_id = ?", id)
	}
	if cid := c.QueryParam("customer_id"); cid != "" {
		id, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			return badRequest(c, "invalid customer_id")
		}
		q = q.Where("project_id IN (?)", h.DB.Model(&models.Project{}).Select("_id").Where("customer_id = ?", id))
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
	if item.ProjectID == 0 {
		return badRequest(c, "project_id is required")
	}
	if item.Start.IsZero() {
		return badRequest(c, "start is required")
	}
	if !h.projectExists(item.ProjectID) {
		return badRequest(c, "project not found")
	}

	// Idempotent create: if client_uuid already exists for this user, return it.
	if item.ClientUUID != nil && *item.ClientUUID != "" {
		var existing models.Report
		err := h.DB.Where("client_uuid = ? AND user_id = ?", *item.ClientUUID, item.UserID).
			First(&existing).Error
		if err == nil {
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

	if err := h.DB.Create(item).Error; err != nil {
		if item.ClientUUID != nil && *item.ClientUUID != "" {
			var existing models.Report
			if err2 := h.DB.Where("client_uuid = ? AND user_id = ?", *item.ClientUUID, item.UserID).
				First(&existing).Error; err2 == nil {
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
	if updated.ProjectID == 0 {
		return badRequest(c, "project_id is required")
	}
	if !h.projectExists(updated.ProjectID) {
		return badRequest(c, "project not found")
	}

	seq, err := database.NextReportSeq(h.DB)
	if err != nil {
		return internal(c, "failed to allocate seq")
	}
	updated.Seq = seq
	updated.Modified = time.Now().UTC()
	updated.ID = id

	if err := h.DB.Save(updated).Error; err != nil {
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
	ClientUUID string `json:"client_uuid"`
	ProjectID  int64  `json:"project_id"`
	Start      string `json:"start"`
	Stop       string `json:"stop"`
	Comment    string `json:"comment"`
	UserID     int64  `json:"user_id"`
}

// BatchUpdateItem is one existing report update in a batch push.
type BatchUpdateItem struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	Start      string `json:"start"`
	Stop       string `json:"stop"`
	Comment    string `json:"comment"`
	ClientUUID string `json:"client_uuid"`
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
		}
		if item.UserID == 0 || !isAdmin {
			item.UserID = selfID
		}
		if item.ProjectID == 0 {
			return badRequest(c, "create: project_id is required")
		}
		if !h.projectExists(item.ProjectID) {
			return badRequest(c, "create: project not found")
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
				out.Created = append(out.Created, existing)
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
		if err := h.DB.Create(item).Error; err != nil {
			if item.ClientUUID != nil {
				var existing models.Report
				if err2 := h.DB.Where("client_uuid = ? AND user_id = ?", *item.ClientUUID, item.UserID).
					First(&existing).Error; err2 == nil {
					out.Created = append(out.Created, existing)
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
			if !h.projectExists(in.ProjectID) {
				return badRequest(c, "update: project not found")
			}
			existing.ProjectID = in.ProjectID
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
		if err := h.DB.Save(&existing).Error; err != nil {
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
	monthStr := c.QueryParam("month")
	if monthStr == "" {
		monthStr = time.Now().Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", monthStr, time.Local)
	if err != nil {
		return badRequest(c, "month must be YYYY-MM")
	}
	end := start.AddDate(0, 1, 0)

	// Load projects (optionally filtered), then group by customer.
	pq := h.DB.Model(&models.Project{}).Order("name ASC")
	if projectID > 0 {
		pq = pq.Where("_id = ?", projectID)
	}
	if customerID > 0 {
		pq = pq.Where("customer_id = ?", customerID)
	}
	var projects []models.Project
	if err := pq.Find(&projects).Error; err != nil {
		return internal(c, "failed to load projects")
	}

	custIDs := make([]int64, 0)
	seenCust := map[int64]bool{}
	for _, p := range projects {
		if !seenCust[p.CustomerID] {
			seenCust[p.CustomerID] = true
			custIDs = append(custIDs, p.CustomerID)
		}
	}
	custByID := map[int64]models.Customer{}
	if len(custIDs) > 0 {
		var customers []models.Customer
		if err := h.DB.Where("_id IN ?", custIDs).Find(&customers).Error; err != nil {
			return internal(c, "failed to load customers")
		}
		for _, cu := range customers {
			custByID[cu.ID] = cu
		}
	}

	// Group projects by customer
	projectsByCust := map[int64][]models.Project{}
	for _, p := range projects {
		projectsByCust[p.CustomerID] = append(projectsByCust[p.CustomerID], p)
	}

	resp := dto.MonthlyReportResponse{
		Month:      monthStr,
		UserID:     userID,
		CustomerID: customerID,
		ProjectID:  projectID,
		Customers:  []dto.MonthlyCustomer{},
	}

	var grandTotal time.Duration

	// Stable customer order by name
	type custKey struct {
		id   int64
		name string
	}
	keys := make([]custKey, 0, len(projectsByCust))
	for cid := range projectsByCust {
		name := ""
		if cu, ok := custByID[cid]; ok {
			name = cu.Name
		}
		keys = append(keys, custKey{id: cid, name: name})
	}
	// simple insertion sort by name
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && keys[j].name < keys[j-1].name {
			keys[j], keys[j-1] = keys[j-1], keys[j]
			j--
		}
	}

	for _, ck := range keys {
		cu := custByID[ck.id]
		mc := dto.MonthlyCustomer{
			CustomerID:  cu.ID,
			Name:        cu.Name,
			Description: cu.Description,
			Projects:    []dto.MonthlyProject{},
		}
		var custTotal time.Duration

		for _, proj := range projectsByCust[ck.id] {
			var reports []models.Report
			err := h.DB.Where(
				"user_id = ? AND project_id = ? AND start >= ? AND start < ? AND deleted = ?",
				userID, proj.ID, start, end, false,
			).Order("start ASC").Find(&reports).Error
			if err != nil {
				return internal(c, "failed to load reports")
			}
			if len(reports) == 0 {
				continue
			}

			mp := dto.MonthlyProject{
				ProjectID:  proj.ID,
				Name:       proj.Name,
				CostCenter: proj.CostCenter,
				Days:       []dto.MonthlyDay{},
			}
			var projTotal time.Duration
			var curDay *dto.MonthlyDay
			var curDate string

			for _, r := range reports {
				if r.Stop == nil {
					continue
				}
				dur := r.Stop.Sub(r.Start)
				if dur < 0 {
					continue
				}
				dateStr := r.Start.Format("2006-01-02")
				if curDay == nil || dateStr != curDate {
					if curDay != nil {
						mp.Days = append(mp.Days, *curDay)
					}
					curDate = dateStr
					curDay = &dto.MonthlyDay{Date: dateStr, Entries: []dto.MonthlyEntry{}}
				}
				entry := dto.MonthlyEntry{
					ID:      r.ID,
					Start:   r.Start.Format("15:04"),
					Stop:    r.Stop.Format("15:04"),
					Hours:   dur.Hours(),
					HM:      formatHM(dur),
					Comment: r.Comment,
				}
				curDay.Entries = append(curDay.Entries, entry)
				curDay.TotalHours += dur.Hours()
				projTotal += dur
			}
			if curDay != nil {
				mp.Days = append(mp.Days, *curDay)
			}
			for i := range mp.Days {
				mp.Days[i].TotalHM = formatHM(time.Duration(mp.Days[i].TotalHours * float64(time.Hour)))
			}
			mp.TotalHours = projTotal.Hours()
			mp.TotalHM = formatHM(projTotal)
			custTotal += projTotal
			mc.Projects = append(mc.Projects, mp)
		}

		if len(mc.Projects) == 0 {
			continue
		}
		mc.TotalHours = custTotal.Hours()
		mc.TotalHM = formatHM(custTotal)
		grandTotal += custTotal
		resp.Customers = append(resp.Customers, mc)
	}

	resp.TotalHours = grandTotal.Hours()
	resp.TotalHM = formatHM(grandTotal)
	return ok(c, resp)
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
