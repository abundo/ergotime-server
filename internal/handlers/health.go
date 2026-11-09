package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// HealthHandler provides liveness/readiness probes.
type HealthHandler struct {
	DB *gorm.DB
}

func (h *HealthHandler) Live(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Ready(c echo.Context) error {
	sqlDB, err := h.DB.DB()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db error"})
	}
	if err := sqlDB.Ping(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db unreachable"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}
