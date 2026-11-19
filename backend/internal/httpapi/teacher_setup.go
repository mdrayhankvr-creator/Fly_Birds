package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/schoolsetup"
)

type teacherSetupHandler struct{ services *AdminServices }

func (h *teacherSetupHandler) ready(c *gin.Context) bool {
	if h.services == nil || h.services.TeacherSetupDB == nil {
		failure(c, http.StatusServiceUnavailable, "setup_unavailable", "teacher setup requires a database")
		return false
	}
	return true
}

func teacherSetupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, schoolsetup.ErrTeacherSetupForbidden):
		failure(c, 403, "permission_denied", "teacher setup is not allowed")
	case errors.Is(err, schoolsetup.ErrTeacherSetupInvalid):
		failure(c, 400, "invalid_request", "invalid teacher setup request")
	case errors.Is(err, schoolsetup.ErrTeacherSetupStale):
		failure(c, 409, "setup_stale", "school staff changed; preview again")
	case errors.Is(err, schoolsetup.ErrTeacherSetupKeyReused):
		failure(c, 409, "idempotency_key_reused", "request key was used for another preview")
	case errors.Is(err, schoolsetup.ErrTeacherSetupConflict):
		failure(c, 409, "setup_conflict", "teacher identifiers or protected accounts require review")
	default:
		failure(c, 500, "internal_error", "teacher setup failed")
	}
}

func (h *teacherSetupHandler) preview(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	plan, err := schoolsetup.PreviewTeachers(c.Request.Context(), h.services.TeacherSetupDB, adminUser(c).ID)
	if err != nil {
		teacherSetupError(c, err)
		return
	}
	success(c, 200, plan)
}

func (h *teacherSetupHandler) apply(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.ready(c) {
		return
	}
	var request schoolsetup.TeacherApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		teacherSetupError(c, schoolsetup.ErrTeacherSetupInvalid)
		return
	}
	result, err := schoolsetup.ApplyTeachers(c.Request.Context(), h.services.TeacherSetupDB, adminUser(c).ID, request)
	if err != nil {
		teacherSetupError(c, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	success(c, status, result)
}
