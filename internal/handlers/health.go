package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// SyncProtocol is the desktop sync contract advertised on /healthz.
// Leave it unchanged when a release only adds fields. Bump it when an
// older client would misread this server or this server would reject
// that client: a removed or redefined field, a new timestamp format,
// or a different auth scheme.
// Protocol 2 adds report lines (several project comments on one customer
// interval). An older client would store only project_id and comment, then
// push that back and drop the other lines.
// Protocol 3 rejects a report whose start or stop is not a whole minute, or
// whose closed length is under one minute. The previous client sent seconds
// from the tracker, so this server would reject that client's pushes.
const SyncProtocol = 3

// HealthHandler provides liveness/readiness probes.
type HealthHandler struct {
	DB *gorm.DB
}

func (h *HealthHandler) Live(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"status":   "ok",
		"protocol": SyncProtocol,
	})
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
