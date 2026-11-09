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

// CustomersHandler manages customers.
type CustomersHandler struct {
	DB *gorm.DB
}

func (h *CustomersHandler) List(c echo.Context) error {
	q := h.DB.Model(&models.Customer{}).Order("active DESC, name ASC")
	if c.QueryParam("active") == "1" || c.QueryParam("active") == "true" {
		q = q.Where("active = ?", true)
	}
	var items []models.Customer
	if err := q.Find(&items).Error; err != nil {
		return internal(c, "failed to list customers")
	}
	return okData(c, items)
}

func (h *CustomersHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var item models.Customer
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "customer not found")
		}
		return internal(c, "database error")
	}
	return ok(c, item)
}

func (h *CustomersHandler) Create(c echo.Context) error {
	var req dto.CustomerRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return badRequest(c, "name is required")
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	item := models.Customer{
		Name:        req.Name,
		Description: strings.TrimSpace(req.Description),
		Active:      active,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		return internal(c, "failed to create customer")
	}
	return created(c, item)
}

func (h *CustomersHandler) Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var item models.Customer
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "customer not found")
		}
		return internal(c, "database error")
	}

	var req dto.CustomerRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		item.Name = name
	}
	item.Description = strings.TrimSpace(req.Description)
	if req.Active != nil {
		item.Active = *req.Active
	}

	if err := h.DB.Save(&item).Error; err != nil {
		return internal(c, "failed to update customer")
	}
	return ok(c, item)
}

func (h *CustomersHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}

	var projectCount int64
	if err := h.DB.Model(&models.Project{}).Where("customer_id = ?", id).Count(&projectCount).Error; err != nil {
		return internal(c, "database error")
	}
	if projectCount > 0 {
		// Soft-deactivate when projects exist
		if err := h.DB.Model(&models.Customer{}).Where("_id = ?", id).Update("active", false).Error; err != nil {
			return internal(c, "failed to deactivate customer")
		}
		return ok(c, map[string]string{"message": "customer deactivated (has projects)"})
	}

	res := h.DB.Delete(&models.Customer{}, id)
	if res.Error != nil {
		return internal(c, "failed to delete customer")
	}
	if res.RowsAffected == 0 {
		return notFound(c, "customer not found")
	}
	return c.NoContent(http.StatusNoContent)
}
