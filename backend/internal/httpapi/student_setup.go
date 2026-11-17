package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/schoolsetup"
)

type studentSetupHandler struct{ services *AdminServices }

func (h *studentSetupHandler) ready(c *gin.Context) bool {
	if h.services == nil || h.services.StudentSetupDB == nil {
		failure(c, http.StatusServiceUnavailable, "setup_unavailable", "student setup requires a database")
		return false
	}
	return true
}

func studentSetupError(c *gin.Context, err error) {
	var conflict *schoolsetup.StudentSetupConflictError
	if errors.As(err, &conflict) {
		c.JSON(409, gin.H{"error": gin.H{"code": "setup_conflict", "message": "school setup has conflicts; review the listed identifiers", "conflicts": conflict.Conflicts}})
		return
	}
	switch {
	case errors.Is(err, schoolsetup.ErrStudentSetupForbidden):
		failure(c, 403, "permission_denied", "student setup is not allowed")
	case errors.Is(err, schoolsetup.ErrStudentSetupInvalid):
		failure(c, 400, "invalid_request", "invalid student setup request")
	case errors.Is(err, schoolsetup.ErrStudentSetupStale):
		failure(c, 409, "setup_stale", "school state changed; preview again")
	case errors.Is(err, schoolsetup.ErrStudentSetupKeyReused):
		failure(c, 409, "idempotency_key_reused", "request key was used for another preview")
	case errors.Is(err, schoolsetup.ErrStudentSetupConflict):
		// A manifest collision or unsafe school state is reviewable; no write
		// has occurred. Do not expose database internals in the response.
		failure(c, 409, "setup_conflict", "school setup has conflicting or incomplete identifiers; review the school state")
	default:
		failure(c, 500, "internal_error", "student setup failed")
	}
}

func (h *studentSetupHandler) preview(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	plan, err := schoolsetup.PreviewStudents(c.Request.Context(), h.services.StudentSetupDB, adminUser(c).ID, c.Query("session_id"))
	if err != nil {
		studentSetupError(c, err)
		return
	}
	success(c, 200, plan)
}

func (h *studentSetupHandler) apply(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var request schoolsetup.StudentApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		studentSetupError(c, schoolsetup.ErrStudentSetupInvalid)
		return
	}
	result, err := schoolsetup.ApplyStudents(c.Request.Context(), h.services.StudentSetupDB, adminUser(c).ID, request)
	if err != nil {
		studentSetupError(c, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	success(c, status, result)
}
