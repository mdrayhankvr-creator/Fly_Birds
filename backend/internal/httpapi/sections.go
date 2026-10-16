package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type sectionsHandler struct{ service *academic.SectionService }

func (h *sectionsHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "academic_unavailable", "sections require a database")
	return false
}

func (h *sectionsHandler) list(c *gin.Context) {
	if !h.available(c) {
		return
	}
	q, err := academicQuery(c)
	if err != nil {
		academicError(c, academic.ErrInvalid)
		return
	}
	value, err := h.service.List(c.Request.Context(), academic.SectionQuery{Query: q, SessionID: c.Query("session_id"), ClassID: c.Query("class_id")})
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *sectionsHandler) get(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		academicError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *sectionsHandler) save(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input academic.SectionInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	actor := c.MustGet("auth_user").(auth.User)
	value, err := h.service.Save(c.Request.Context(), actor.ID, c.Param("id"), input)
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
