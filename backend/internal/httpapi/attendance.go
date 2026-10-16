package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type attendanceHandler struct{ service *attendance.Service }

// attendanceSessions godoc
// @Summary List academic sessions available to attendance users
// @Tags attendance
// @Router /attendance/sessions [get]
func (h *attendanceHandler) attendanceSessions(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.ListSessions(c.Request.Context(), attendance.Query{Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *attendanceHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "attendance_unavailable", "attendance requires a database")
	return false
}
func attendanceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, attendance.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid attendance input")
	case errors.Is(err, attendance.ErrNotFound):
		failure(c, 404, "not_found", "attendance resource not found")
	case errors.Is(err, attendance.ErrDuplicate):
		failure(c, 409, "duplicate_attendance", "attendance already recorded")
	case errors.Is(err, attendance.ErrConflict):
		failure(c, 409, "attendance_conflict", "attendance relationship or status conflict")
	case errors.Is(err, attendance.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func attendanceActor(c *gin.Context) string { return c.MustGet("auth_user").(auth.User).ID }

// teacherRoster godoc
// @Summary Read the daily teacher roster with recorded statuses
// @Tags attendance
// @Router /attendance/teachers/roster [get]
func (h *attendanceHandler) teacherRoster(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.Roster(c.Request.Context(), attendance.Query{SessionID: c.Query("session_id"), Date: c.Query("date"), Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

// recordTeacher godoc
// @Summary Record one teacher's attendance for a date
// @Tags attendance
// @Router /attendance/teachers [post]
func (h *attendanceHandler) recordTeacher(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input attendance.RecordInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	v, err := h.service.Record(c.Request.Context(), attendanceActor(c), input)
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, http.StatusCreated, v)
}

// correctTeacher godoc
// @Summary Correct an existing teacher attendance record
// @Tags attendance
// @Router /attendance/teachers/{id} [patch]
func (h *attendanceHandler) correctTeacher(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input attendance.CorrectionInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	v, err := h.service.Correct(c.Request.Context(), attendanceActor(c), c.Param("id"), input)
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

// ownTeacherHistory godoc
// @Summary Read own teacher attendance history
// @Tags attendance
// @Router /attendance/teachers/me [get]
func (h *attendanceHandler) ownTeacherHistory(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.OwnHistory(c.Request.Context(), attendanceActor(c), attendance.Query{SessionID: c.Query("session_id"), From: c.Query("from"), To: c.Query("to"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

// teacherMonthly godoc
// @Summary Read paginated monthly teacher attendance totals
// @Tags attendance
// @Router /attendance/teachers/monthly [get]
func (h *attendanceHandler) teacherMonthly(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.Monthly(c.Request.Context(), attendance.Query{SessionID: c.Query("session_id"), Month: c.Query("month"), Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

// ownTeacherMonthly godoc
// @Summary Read own monthly teacher attendance totals
// @Tags attendance
// @Router /attendance/teachers/me/monthly [get]
func (h *attendanceHandler) ownTeacherMonthly(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.OwnMonthly(c.Request.Context(), attendanceActor(c), attendance.Query{SessionID: c.Query("session_id"), Month: c.Query("month")})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}
