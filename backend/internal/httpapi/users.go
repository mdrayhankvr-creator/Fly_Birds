package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
)

type usersHandler struct{ service *users.Service }

func (h *usersHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "users_unavailable", "user management requires a database")
	return false
}
func usersError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, users.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid user input")
	case errors.Is(err, users.ErrNotFound):
		failure(c, 404, "not_found", "user not found")
	case errors.Is(err, users.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	case errors.Is(err, users.ErrDuplicate):
		failure(c, 409, "duplicate_email", "email already registered")
	case errors.Is(err, users.ErrLastAdmin):
		failure(c, 409, "last_admin", "an authorized administrator must remain")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

// listUsers godoc
// @Summary List and filter users
// @Tags users
// @Router /users [get]
func (h *usersHandler) list(c *gin.Context) {
	if !h.available(c) {
		return
	}
	search, page, size, err := pageParams(c)
	if err != nil {
		usersError(c, users.ErrInvalid)
		return
	}
	value, err := h.service.List(c.Request.Context(), users.Query{Search: search, Role: c.Query("role"), Status: c.Query("status"), Page: page, PageSize: size})
	if err != nil {
		usersError(c, err)
		return
	}
	success(c, 200, value)
}

// createUser godoc
// @Summary Create a user with an administrator supplied initial password
// @Tags users
// @Router /users [post]
func (h *usersHandler) create(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input users.CreateInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.Create(c.Request.Context(), actor.ID, input)
	if err != nil {
		usersError(c, err)
		return
	}
	success(c, http.StatusCreated, value)
}

// getUser godoc
// @Summary Get a user without credentials or session details
// @Tags users
// @Router /users/{id} [get]
func (h *usersHandler) get(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		usersError(c, err)
		return
	}
	success(c, 200, value)
}

// updateUser godoc
// @Summary Update allowlisted user profile fields
// @Tags users
// @Router /users/{id} [patch]
func (h *usersHandler) update(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input users.UpdateInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.Update(c.Request.Context(), actor.ID, c.Param("id"), input)
	if err != nil {
		usersError(c, err)
		return
	}
	success(c, 200, value)
}

// updateUserStatus godoc
// @Summary Activate or deactivate a user and revoke sessions on deactivation
// @Tags users
// @Router /users/{id}/status [patch]
func (h *usersHandler) status(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input struct {
		IsActive *bool `json:"is_active"`
	}
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	if input.IsActive == nil {
		usersError(c, users.ErrInvalid)
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.SetStatus(c.Request.Context(), actor.ID, c.Param("id"), *input.IsActive)
	if err != nil {
		usersError(c, err)
		return
	}
	success(c, 200, value)
}

func parsePage(c *gin.Context) (int, int, error) {
	page, size := 1, 50
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, err
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, err
		}
	}
	return page, size, nil
}
