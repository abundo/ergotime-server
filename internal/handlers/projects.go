package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/abundo/ergotime-server/internal/dto"
	"github.com/abundo/ergotime-server/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ProjectsHandler manages projects under customers.
type ProjectsHandler struct {
	DB *gorm.DB
}

func (h *ProjectsHandler) List(c echo.Context) error {
	q := h.DB.Model(&models.Project{}).Order("name ASC")
	if c.QueryParam("active") == "1" || c.QueryParam("active") == "true" {
		q = q.Where("active = ?", true)
	}
	if cid := c.QueryParam("customer_id"); cid != "" {
		id, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			return badRequest(c, "invalid customer_id")
		}
		q = q.Where("customer_id = ?", id)
	}
	var items []models.Project
	if err := q.Find(&items).Error; err != nil {
		return internal(c, "failed to list projects")
	}
	return okData(c, items)
}

func (h *ProjectsHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var item models.Project
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "project not found")
		}
		return internal(c, "database error")
	}
	return ok(c, item)
}

func (h *ProjectsHandler) Create(c echo.Context) error {
	var req dto.ProjectRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return badRequest(c, "name is required")
	}
	if req.CustomerID <= 0 {
		return badRequest(c, "customer_id is required")
	}
	var cust models.Customer
	if err := h.DB.First(&cust, req.CustomerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return badRequest(c, "customer not found")
		}
		return internal(c, "database error")
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	unit := strings.TrimSpace(req.Unit)
	if unit == "" {
		unit = "hours"
	}
	item := models.Project{
		CustomerID: req.CustomerID,
		Name:       req.Name,
		CostCenter: strings.TrimSpace(req.CostCenter),
		Unit:       unit,
		Active:     active,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		return internal(c, "failed to create project")
	}
	return created(c, item)
}

func (h *ProjectsHandler) Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var item models.Project
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "project not found")
		}
		return internal(c, "database error")
	}

	var req dto.ProjectRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		item.Name = name
	}
	item.CostCenter = strings.TrimSpace(req.CostCenter)
	if u := strings.TrimSpace(req.Unit); u != "" {
		item.Unit = u
	}
	if req.CustomerID > 0 && req.CustomerID != item.CustomerID {
		var cust models.Customer
		if err := h.DB.First(&cust, req.CustomerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return badRequest(c, "customer not found")
			}
			return internal(c, "database error")
		}
		item.CustomerID = req.CustomerID
	}
	if req.Active != nil {
		item.Active = *req.Active
	}

	if err := h.DB.Save(&item).Error; err != nil {
		return internal(c, "failed to update project")
	}
	return ok(c, item)
}

func (h *ProjectsHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}

	var reportCount int64
	if err := h.DB.Model(&models.Report{}).Where("project_id = ?", id).Count(&reportCount).Error; err != nil {
		return internal(c, "database error")
	}
	if reportCount > 0 {
		if err := h.DB.Model(&models.Project{}).Where("_id = ?", id).Update("active", false).Error; err != nil {
			return internal(c, "failed to deactivate project")
		}
		return ok(c, map[string]string{"message": "project deactivated (has reports)"})
	}

	res := h.DB.Delete(&models.Project{}, id)
	if res.Error != nil {
		return internal(c, "failed to delete project")
	}
	if res.RowsAffected == 0 {
		return notFound(c, "project not found")
	}
	return c.NoContent(http.StatusNoContent)
}
