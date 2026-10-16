package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type academicHandler struct{ service *academic.Service }

func (h *academicHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "academic_unavailable", "academic structure requires a database")
	return false
}
func academicError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, academic.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid academic input")
	case errors.Is(err, academic.ErrNotFound):
		failure(c, 404, "not_found", "academic record not found")
	case errors.Is(err, academic.ErrDuplicate):
		failure(c, 409, "duplicate_academic_record", "academic record already exists")
	case errors.Is(err, academic.ErrInactive):
		failure(c, 409, "inactive_academic_record", "class and subject must be active")
	case errors.Is(err, academic.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func academicQuery(c *gin.Context) (academic.Query, error) {
	page, size, err := parsePage(c)
	return academic.Query{Search: c.Query("search"), Status: c.Query("status"), Page: page, PageSize: size}, err
}
func (h *academicHandler) classes(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := academicQuery(c)
	if err != nil {
		academicError(c, academic.ErrInvalid)
		return
	}
	value, err := h.service.ListClasses(c.Request.Context(), query)
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}
func (h *academicHandler) class(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.GetClass(c.Request.Context(), c.Param("id"))
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}
func (h *academicHandler) saveClass(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academic.ClassInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.SaveClass(c.Request.Context(), actor.ID, c.Param("id"), input)
	if err != nil {
		academicError(c, err)
		return
	}
	status := http.StatusOK
	if c.Request.Method == http.MethodPost {
		status = http.StatusCreated
	}
	success(c, status, value)
}
func (h *academicHandler) subjects(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := academicQuery(c)
	if err != nil {
		academicError(c, academic.ErrInvalid)
		return
	}
	value, err := h.service.ListSubjects(c.Request.Context(), query)
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}
func (h *academicHandler) subject(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.GetSubject(c.Request.Context(), c.Param("id"))
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}
func (h *academicHandler) saveSubject(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academic.SubjectInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.SaveSubject(c.Request.Context(), actor.ID, c.Param("id"), input)
	if err != nil {
		academicError(c, err)
		return
	}
	status := http.StatusOK
	if c.Request.Method == http.MethodPost {
		status = http.StatusCreated
	}
	success(c, status, value)
}
func (h *academicHandler) mappings(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := academicQuery(c)
	if err != nil {
		academicError(c, academic.ErrInvalid)
		return
	}
	value, err := h.service.ListMappings(c.Request.Context(), c.Param("id"), query)
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}
func (h *academicHandler) createMapping(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academic.MappingInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.CreateMapping(c.Request.Context(), actor.ID, c.Param("id"), input)
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 201, value)
}
func (h *academicHandler) updateMapping(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academic.MappingInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.UpdateMapping(c.Request.Context(), actor.ID, c.Param("id"), c.Param("subjectId"), input)
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}
func (h *academicHandler) deleteMapping(c *gin.Context) {
	if !h.available(c) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	if err := h.service.DeleteMapping(c.Request.Context(), actor.ID, c.Param("id"), c.Param("subjectId")); err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
