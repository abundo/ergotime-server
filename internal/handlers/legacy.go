package handlers

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// acceptsLegacy detects desktop client requests (form posts / Accept quirks).
// SPA and modern clients use application/json and modern field names.
func acceptsLegacy(c echo.Context) bool {
	if c.QueryParam("legacy") == "1" {
		return true
	}
	ua := c.Request().Header.Get("User-Agent")
	return strings.Contains(ua, "python-requests")
}
