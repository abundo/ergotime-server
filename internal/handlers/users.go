package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/abundo/ergotime-server/internal/auth"
	"github.com/abundo/ergotime-server/internal/dto"
	"github.com/abundo/ergotime-server/internal/middleware"
	"github.com/abundo/ergotime-server/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// UsersHandler manages local user accounts (admin).
type UsersHandler struct {
	DB *gorm.DB
}

func (h *UsersHandler) List(c echo.Context) error {
	var users []models.User
	if err := h.DB.Order("name ASC").Find(&users).Error; err != nil {
		return internal(c, "failed to list users")
	}
	out := make([]dto.UserPublic, 0, len(users))
	for i := range users {
		out = append(out, dto.UserFromModel(&users[i]))
	}
	return okData(c, out)
}

func (h *UsersHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "user not found")
		}
		return internal(c, "database error")
	}
	return ok(c, dto.UserFromModel(&user))
}

func (h *UsersHandler) Create(c echo.Context) error {
	var req dto.CreateUserRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return badRequest(c, "name is required")
	}
	if len(req.Password) < 6 {
		return badRequest(c, "password must be at least 6 characters")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return badRequest(c, err.Error())
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}
	isAdmin := false
	if req.IsAdmin != nil {
		isAdmin = *req.IsAdmin
	}

	user := models.User{
		Name:         req.Name,
		FullName:     strings.TrimSpace(req.FullName),
		PasswordHash: hash,
		Active:       active,
		IsAdmin:      isAdmin,
	}
	if err := h.DB.Create(&user).Error; err != nil {
		if isUniqueViolation(err) {
			return badRequest(c, "username already exists")
		}
		return internal(c, "failed to create user")
	}
	return created(c, dto.UserFromModel(&user))
}

func (h *UsersHandler) Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}

	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound(c, "user not found")
		}
		return internal(c, "database error")
	}

	var req dto.UpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid request body")
	}

	// Non-admins can only change their own password (and not elevate privileges)
	callerID := middleware.UserIDFromContext(c)
	isAdmin := middleware.IsAdminFromContext(c)
	if !isAdmin && callerID != id {
		return forbidden(c, "cannot modify other users")
	}

	if req.Name != nil {
		if !isAdmin {
			return forbidden(c, "only admins can rename users")
		}
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return badRequest(c, "name cannot be empty")
		}
		user.Name = name
	}
	if req.FullName != nil {
		user.FullName = strings.TrimSpace(*req.FullName)
	}
	if req.Password != nil {
		if len(*req.Password) < 6 {
			return badRequest(c, "password must be at least 6 characters")
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			return badRequest(c, err.Error())
		}
		user.PasswordHash = hash
	}
	if req.Active != nil {
		if !isAdmin {
			return forbidden(c, "only admins can change active flag")
		}
		user.Active = *req.Active
	}
	if req.IsAdmin != nil {
		if !isAdmin {
			return forbidden(c, "only admins can change admin flag")
		}
		user.IsAdmin = *req.IsAdmin
	}

	if err := h.DB.Save(&user).Error; err != nil {
		if isUniqueViolation(err) {
			return badRequest(c, "username already exists")
		}
		return internal(c, "failed to update user")
	}
	return ok(c, dto.UserFromModel(&user))
}

func (h *UsersHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	callerID := middleware.UserIDFromContext(c)
	if callerID == id {
		return badRequest(c, "cannot delete your own account")
	}

	res := h.DB.Delete(&models.User{}, id)
	if res.Error != nil {
		return internal(c, "failed to delete user")
	}
	if res.RowsAffected == 0 {
		return notFound(c, "user not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "duplicate")
}
