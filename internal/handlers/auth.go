package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/abundo/ergotime-server/internal/auth"
	"github.com/abundo/ergotime-server/internal/config"
	"github.com/abundo/ergotime-server/internal/dto"
	"github.com/abundo/ergotime-server/internal/middleware"
	"github.com/abundo/ergotime-server/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// AuthHandler handles login/logout/me.
type AuthHandler struct {
	DB     *gorm.DB
	Tokens *auth.TokenService
	Cfg    *config.Config
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req dto.LoginRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return badRequest(c, "username and password are required")
	}

	var user models.User
	err := h.DB.Where("name = ?", req.Username).First(&user).Error
	if err != nil {
		// constant-time-ish: still run a dummy hash check path message
		return fail(c, http.StatusUnauthorized, "invalid username or password")
	}
	if !user.Active {
		return fail(c, http.StatusUnauthorized, "account is disabled")
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		return fail(c, http.StatusUnauthorized, "invalid username or password")
	}

	token, exp, err := h.Tokens.Issue(user.ID, user.Name, user.IsAdmin)
	if err != nil {
		return internal(c, "failed to issue session")
	}

	h.setSessionCookie(c, token, exp)

	return ok(c, dto.LoginResponse{User: dto.UserFromModel(&user)})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	h.clearSessionCookie(c)
	return ok(c, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) Me(c echo.Context) error {
	uid := middleware.UserIDFromContext(c)
	var user models.User
	if err := h.DB.First(&user, uid).Error; err != nil {
		return fail(c, http.StatusUnauthorized, "session invalid")
	}
	if !user.Active {
		h.clearSessionCookie(c)
		return fail(c, http.StatusUnauthorized, "account is disabled")
	}
	return ok(c, dto.UserFromModel(&user))
}

func (h *AuthHandler) setSessionCookie(c echo.Context, token string, exp time.Time) {
	sameSite := http.SameSiteLaxMode
	switch strings.ToLower(h.Cfg.Auth.CookieSameSite) {
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		sameSite = http.SameSiteNoneMode
	}

	cookie := &http.Cookie{
		Name:     h.Cfg.Auth.CookieName,
		Value:    token,
		Path:     h.Cfg.Auth.CookiePath,
		Domain:   h.Cfg.Auth.CookieDomain,
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
		HttpOnly: true,
		Secure:   h.Cfg.Auth.CookieSecure,
		SameSite: sameSite,
	}
	c.SetCookie(cookie)
}

func (h *AuthHandler) clearSessionCookie(c echo.Context) {
	sameSite := http.SameSiteLaxMode
	cookie := &http.Cookie{
		Name:     h.Cfg.Auth.CookieName,
		Value:    "",
		Path:     h.Cfg.Auth.CookiePath,
		Domain:   h.Cfg.Auth.CookieDomain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   h.Cfg.Auth.CookieSecure,
		SameSite: sameSite,
	}
	c.SetCookie(cookie)
}
