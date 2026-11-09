package handlers

import (
	"net/http"

	"github.com/abundo/ergotime-server/internal/dto"
	"github.com/labstack/echo/v4"
)

func ok(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, data)
}

func okData(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, dto.Envelope{Data: data})
}

func created(c echo.Context, data any) error {
	return c.JSON(http.StatusCreated, data)
}

func fail(c echo.Context, status int, msg string) error {
	return c.JSON(status, dto.ErrorBody{Message: msg})
}

func badRequest(c echo.Context, msg string) error {
	return fail(c, http.StatusBadRequest, msg)
}

func notFound(c echo.Context, msg string) error {
	return fail(c, http.StatusNotFound, msg)
}

func forbidden(c echo.Context, msg string) error {
	return fail(c, http.StatusForbidden, msg)
}

func internal(c echo.Context, msg string) error {
	return fail(c, http.StatusInternalServerError, msg)
}
