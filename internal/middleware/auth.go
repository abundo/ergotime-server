package middleware

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/abundo/ergotime-server/internal/auth"
	"github.com/abundo/ergotime-server/internal/config"
	"github.com/abundo/ergotime-server/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	ContextUserID   = "user_id"
	ContextUserName = "user_name"
	ContextIsAdmin  = "is_admin"
	ContextClaims   = "claims"
)

// AuthRequired validates JWT (cookie or Bearer) or HTTP Basic against the users table.
func AuthRequired(tokens *auth.TokenService, cfg *config.Config, db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1) JWT cookie / Bearer. The signature is not enough: the account
			// must still exist and be active, and admin rights come from
			// the row so a demotion takes effect before the token expires.
			if tokenStr := extractBearerOrCookie(c, cfg.Auth.CookieName); tokenStr != "" {
				claims, err := tokens.Parse(tokenStr)
				if err == nil {
					var u models.User
					if err := db.Select("_id", "name", "active", "is_admin").First(&u, claims.UserID).Error; err == nil && u.Active {
						c.Set(ContextUserID, u.ID)
						c.Set(ContextUserName, u.Name)
						c.Set(ContextIsAdmin, u.IsAdmin)
						c.Set(ContextClaims, claims)
						return next(c)
					}
				}
			}

			// 2) HTTP Basic (desktop clients / scripts)
			if user, pass, ok := parseBasicAuth(c.Request().Header.Get(echo.HeaderAuthorization)); ok {
				var u models.User
				if err := db.Where("name = ?", user).First(&u).Error; err == nil && u.Active {
					if auth.CheckPassword(u.PasswordHash, pass) {
						c.Set(ContextUserID, u.ID)
						c.Set(ContextUserName, u.Name)
						c.Set(ContextIsAdmin, u.IsAdmin)
						return next(c)
					}
				}
			}

			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
	}
}

// AdminRequired must run after AuthRequired.
func AdminRequired() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			isAdmin, _ := c.Get(ContextIsAdmin).(bool)
			if !isAdmin {
				return echo.NewHTTPError(http.StatusForbidden, "admin privileges required")
			}
			return next(c)
		}
	}
}

func extractBearerOrCookie(c echo.Context, cookieName string) string {
	authz := c.Request().Header.Get(echo.HeaderAuthorization)
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return strings.TrimSpace(authz[7:])
	}
	if cookie, err := c.Cookie(cookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

func parseBasicAuth(header string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// UserIDFromContext returns the authenticated user id.
func UserIDFromContext(c echo.Context) int64 {
	id, _ := c.Get(ContextUserID).(int64)
	return id
}

// IsAdminFromContext returns whether the authenticated user is admin.
func IsAdminFromContext(c echo.Context) bool {
	v, _ := c.Get(ContextIsAdmin).(bool)
	return v
}
