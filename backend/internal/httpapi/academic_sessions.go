package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academicsessions"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type academicSessionsHandler struct{ service *academicsessions.Service }

func (h *academicSessionsHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "academic_sessions_unavailable", "academic sessions require a database")
	return false
}
func academicSessionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, academicsessions.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid academic session")
	case errors.Is(err, academicsessions.ErrNotFound):
		failure(c, 404, "not_found", "academic session not found")
	case errors.Is(err, academicsessions.ErrDuplicate):
		failure(c, 409, "duplicate_session_name", "academic session name already exists")
	case errors.Is(err, academicsessions.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

// listAcademicSessions godoc
// @Summary List academic sessions with pagination and name search
// @Tags academic-sessions
// @Router /academic/sessions [get]
func (h *academicSessionsHandler) list(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		academicSessionError(c, academicsessions.ErrInvalid)
		return
	}
	result, err := h.service.List(c.Request.Context(), academicsessions.Query{Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		academicSessionError(c, err)
		return
	}
	success(c, 200, result)
}

// currentAcademicSession godoc
// @Summary Get the current active academic session
// @Tags academic-sessions
// @Router /academic/sessions/current [get]
func (h *academicSessionsHandler) current(c *gin.Context) {
	if !h.available(c) {
		return
	}
	result, err := h.service.Current(c.Request.Context())
	if err != nil {
		academicSessionError(c, err)
		return
	}
	success(c, 200, result)
}

// getAcademicSession godoc
// @Summary Get an academic session by ID
// @Tags academic-sessions
// @Router /academic/sessions/{id} [get]
func (h *academicSessionsHandler) get(c *gin.Context) {
	if !h.available(c) {
		return
	}
	result, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		academicSessionError(c, err)
		return
	}
	success(c, 200, result)
}

// createAcademicSession godoc
// @Summary Create an inactive academic session
// @Tags academic-sessions
// @Router /academic/sessions [post]
func (h *academicSessionsHandler) create(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academicsessions.Input
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	result, err := h.service.Create(c.Request.Context(), actor.ID, input)
	if err != nil {
		academicSessionError(c, err)
		return
	}
	success(c, http.StatusCreated, result)
}

// updateAcademicSession godoc
// @Summary Edit an academic session name and dates
// @Tags academic-sessions
// @Router /academic/sessions/{id} [patch]
func (h *academicSessionsHandler) update(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academicsessions.Input
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	result, err := h.service.Update(c.Request.Context(), actor.ID, c.Param("id"), input)
	if err != nil {
		academicSessionError(c, err)
		return
	}
	success(c, 200, result)
}

// setAcademicSessionActivation godoc
// @Summary Activate or deactivate an academic session
// @Tags academic-sessions
// @Router /academic/sessions/{id}/activation [patch]
func (h *academicSessionsHandler) activation(c *gin.Context) {
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
		academicSessionError(c, academicsessions.ErrInvalid)
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	result, err := h.service.SetActive(c.Request.Context(), actor.ID, c.Param("id"), *input.IsActive)
	if err != nil {
		academicSessionError(c, err)
		return
	}
	success(c, 200, result)
}
