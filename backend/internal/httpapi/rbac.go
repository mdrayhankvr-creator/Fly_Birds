package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
)

type rbacHandler struct{ service *rbac.Service }
type assignments struct {
	IDs []string `json:"ids"`
}

func RequireAdministrator() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("auth_user")
		if !ok {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		if current, valid := user.(auth.User); !valid || current.Role != "admin" {
			failure(c, http.StatusForbidden, "forbidden", "permission denied")
			c.Abort()
			return
		}
		c.Next()
	}
}
func (h *rbacHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, http.StatusServiceUnavailable, "auth_unavailable", "authorization requires a configured database")
	return false
}
func rbacError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, rbac.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid role, permission, or assignment")
	case errors.Is(err, rbac.ErrNotFound):
		failure(c, 404, "not_found", "resource not found")
	case errors.Is(err, rbac.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	case errors.Is(err, rbac.ErrLastAdmin):
		failure(c, 409, "last_admin", "an authorized administrator must remain")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func pageParams(c *gin.Context) (string, int, int, error) {
	page, size := 1, 50
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			return "", 0, 0, rbac.ErrInvalid
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil {
			return "", 0, 0, rbac.ErrInvalid
		}
	}
	return c.Query("search"), page, size, nil
}
func decodeAssignments(c *gin.Context) ([]string, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		failure(c, 415, "unsupported_media_type", "application/json is required")
		return nil, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024))
	decoder.DisallowUnknownFields()
	var body assignments
	if decoder.Decode(&body) != nil {
		failure(c, 400, "invalid_request", "a list of ids is required")
		return nil, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		failure(c, 400, "invalid_request", "one JSON object is required")
		return nil, false
	}
	return body.IDs, true
}

// roles godoc
// @Summary List roles with search and pagination
// @Tags authorization
// @Router /roles [get]
func (h *rbacHandler) roles(c *gin.Context) {
	if !h.available(c) {
		return
	}
	search, page, size, err := pageParams(c)
	if err != nil {
		rbacError(c, err)
		return
	}
	result, err := h.service.Roles(c.Request.Context(), search, page, size)
	if err != nil {
		rbacError(c, err)
		return
	}
	success(c, 200, result)
}

// permissions godoc
// @Summary List defined permissions with search and pagination
// @Tags authorization
// @Router /permissions [get]
func (h *rbacHandler) permissions(c *gin.Context) {
	if !h.available(c) {
		return
	}
	search, page, size, err := pageParams(c)
	if err != nil {
		rbacError(c, err)
		return
	}
	result, err := h.service.Permissions(c.Request.Context(), search, page, size)
	if err != nil {
		rbacError(c, err)
		return
	}
	success(c, 200, result)
}

// rolePermissions godoc
// @Summary Read a role's current permissions
// @Tags authorization
// @Router /roles/{id}/permissions [get]
func (h *rbacHandler) rolePermissions(c *gin.Context) {
	if !h.available(c) {
		return
	}
	ids, err := h.service.RolePermissions(c.Request.Context(), c.Param("id"))
	if err != nil {
		rbacError(c, err)
		return
	}
	success(c, 200, ids)
}

// setRolePermissions godoc
// @Summary Replace a role's permissions atomically
// @Tags authorization
// @Router /roles/{id}/permissions [put]
func (h *rbacHandler) setRolePermissions(c *gin.Context) {
	if !h.available(c) {
		return
	}
	ids, ok := decodeAssignments(c)
	if !ok {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	change, err := h.service.SetRolePermissions(c.Request.Context(), actor.ID, c.Param("id"), ids)
	if err != nil {
		rbacError(c, err)
		return
	}
	success(c, 200, change.After)
}

// userRoles godoc
// @Summary Read a user's assigned role
// @Tags authorization
// @Router /users/{id}/roles [get]
func (h *rbacHandler) userRoles(c *gin.Context) {
	if !h.available(c) {
		return
	}
	ids, err := h.service.UserRoles(c.Request.Context(), c.Param("id"))
	if err != nil {
		rbacError(c, err)
		return
	}
	success(c, 200, ids)
}

// setUserRoles godoc
// @Summary Replace a user's single assigned role atomically
// @Tags authorization
// @Router /users/{id}/roles [put]
func (h *rbacHandler) setUserRoles(c *gin.Context) {
	if !h.available(c) {
		return
	}
	ids, ok := decodeAssignments(c)
	if !ok {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	change, err := h.service.SetUserRoles(c.Request.Context(), actor.ID, c.Param("id"), ids)
	if err != nil {
		rbacError(c, err)
		return
	}
	success(c, 200, change.After)
}
