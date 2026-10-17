package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"net/http"
)

type studentAttendanceHandler struct{ service *attendance.StudentService }

func (h *studentAttendanceHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "attendance_unavailable", "attendance requires a database")
	return false
}
func (h *studentAttendanceHandler) roster(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.Roster(c.Request.Context(), attendanceActor(c), attendance.StudentQuery{SessionID: c.Query("session_id"), SectionID: c.Query("section_id"), Date: c.Query("date"), Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *studentAttendanceHandler) record(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in attendance.StudentInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.Record(c.Request.Context(), attendanceActor(c), in)
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, http.StatusCreated, v)
}
func (h *studentAttendanceHandler) scan(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in attendance.ScanInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.Scan(c.Request.Context(), attendanceActor(c), in)
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, http.StatusCreated, v)
}
func (h *studentAttendanceHandler) correct(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in attendance.StudentCorrection
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.Correct(c.Request.Context(), attendanceActor(c), c.Param("id"), in)
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *studentAttendanceHandler) monthly(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.Monthly(c.Request.Context(), attendanceActor(c), attendance.StudentQuery{SessionID: c.Query("session_id"), SectionID: c.Query("section_id"), Month: c.Query("month"), Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *studentAttendanceHandler) self(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Report(c.Request.Context(), attendanceActor(c), "", c.Query("month"), "self")
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *studentAttendanceHandler) child(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Report(c.Request.Context(), attendanceActor(c), c.Param("id"), c.Query("month"), "guardian")
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *studentAttendanceHandler) alerts(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		attendanceError(c, attendance.ErrInvalid)
		return
	}
	v, err := h.service.Alerts(c.Request.Context(), attendanceActor(c), attendance.StudentQuery{Date: c.Query("date"), Search: c.Query("search"), Page: page, PageSize: size})
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *studentAttendanceHandler) sections(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Sections(c.Request.Context(), attendanceActor(c), c.Query("session_id"))
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, 200, v)
}

// saveRoster godoc
// @Summary Record and correct a section's attendance for one day in one transaction
// @Tags attendance
// @Router /attendance/students/roster [put]
func (h *studentAttendanceHandler) saveRoster(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in attendance.RosterInput
	if !decodeSchoolJSON(c, 128*1024, &in) {
		return
	}
	v, err := h.service.SaveRoster(c.Request.Context(), attendanceActor(c), in)
	if err != nil {
		attendanceError(c, err)
		return
	}
	success(c, http.StatusOK, v)
}
