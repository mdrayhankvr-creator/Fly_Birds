package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

type schoolHandler struct{ service *school.Service }
type gradingRequest struct {
	Bands []school.GradeBand `json:"bands"`
}
type moduleRequest struct {
	Enabled *bool `json:"enabled"`
	Cascade bool  `json:"cascade"`
}

func (h *schoolHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "school_unavailable", "school configuration requires a database")
	return false
}
func schoolError(c *gin.Context, err error) {
	var deps *school.DependencyError
	switch {
	case errors.As(err, &deps):
		c.JSON(409, gin.H{"error": gin.H{"code": "module_dependencies", "message": "module dependencies not satisfied", "kind": deps.Kind, "modules": deps.Modules}})
	case errors.Is(err, school.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid school configuration")
	case errors.Is(err, school.ErrNotFound):
		failure(c, 404, "not_found", "school resource not found")
	case errors.Is(err, school.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	case errors.Is(err, school.ErrCoreModule):
		failure(c, 409, "core_module", "core module cannot be disabled")
	case errors.Is(err, school.ErrSlideLimit):
		failure(c, 409, "slide_limit", "the slideshow already has the maximum number of photos")
	case errors.Is(err, school.ErrModuleDisabled):
		failure(c, 403, "module_disabled", "module is disabled")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func decodeSchoolJSON(c *gin.Context, limit int64, destination any) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		failure(c, 415, "unsupported_media_type", "application/json is required")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	if err != nil {
		failure(c, 400, "invalid_request", "invalid JSON body")
		return false
	}
	if containsJSONNUL(body) {
		failure(c, 400, "invalid_request", "NUL characters are not allowed")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil {
		failure(c, 400, "invalid_request", "invalid JSON body")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		failure(c, 400, "invalid_request", "one JSON object is required")
		return false
	}
	return true
}

// branding godoc
// @Summary Read public school branding without private contact information
// @Tags school
// @Router /school/branding [get]
func (h *schoolHandler) branding(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.Branding(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// settings godoc
// @Summary Read the school profile
// @Tags school
// @Router /school/settings [get]
func (h *schoolHandler) settings(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.Settings(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// saveSettings godoc
// @Summary Replace the school profile and theme
// @Tags school
// @Router /school/settings [put]
func (h *schoolHandler) saveSettings(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input school.SettingsInput
	if !decodeSchoolJSON(c, 16*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.SaveSettings(c.Request.Context(), actor.ID, input)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// grading godoc
// @Summary Read the active grading scheme
// @Tags school
// @Router /school/grading [get]
func (h *schoolHandler) grading(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.Grading(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// saveGrading godoc
// @Summary Replace all active grading bands atomically
// @Tags school
// @Router /school/grading [put]
func (h *schoolHandler) saveGrading(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input gradingRequest
	if !decodeSchoolJSON(c, 16*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.SaveGrading(c.Request.Context(), actor.ID, input.Bands)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// modules godoc
// @Summary List module definitions and live enablement states
// @Tags school
// @Router /school/modules [get]
func (h *schoolHandler) modules(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size := 1, 50
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			schoolError(c, school.ErrInvalid)
			return
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil {
			schoolError(c, school.ErrInvalid)
			return
		}
	}
	value, err := h.service.Modules(c.Request.Context(), c.Query("search"), page, size)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// saveModule godoc
// @Summary Enable or disable an optional school module
// @Tags school
// @Router /school/modules/{key} [put]
func (h *schoolHandler) saveModule(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input moduleRequest
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	if input.Enabled == nil {
		schoolError(c, school.ErrInvalid)
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.SetModule(c.Request.Context(), actor.ID, c.Param("key"), *input.Enabled, input.Cascade)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

// RequireModule follows RequireAuth and RequirePermission. It reads the current
// database state per request so a disabled optional module rejects active sessions.
func RequireModule(service *school.Service, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get("auth_user"); !ok {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		if service == nil {
			failure(c, 503, "school_unavailable", "school configuration requires a database")
			c.Abort()
			return
		}
		enabled, err := service.Enabled(c.Request.Context(), key)
		if err != nil {
			schoolError(c, err)
			c.Abort()
			return
		}
		if !enabled {
			schoolError(c, school.ErrModuleDisabled)
			c.Abort()
			return
		}
		c.Next()
	}
}
