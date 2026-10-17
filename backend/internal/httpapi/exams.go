package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

type examHandler struct{ service *exams.Service }

func (h *examHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "exams_unavailable", "exams require a database")
	return false
}
func examError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, exams.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid examination input")
	case errors.Is(err, exams.ErrNotFound):
		failure(c, 404, "not_found", "examination or result not found")
	case errors.Is(err, exams.ErrConflict):
		failure(c, 409, "exam_conflict", "exam state or marks conflict")
	case errors.Is(err, exams.ErrForbidden):
		failure(c, 403, "forbidden", "examination access denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func examUser(c *gin.Context) auth.User { return c.MustGet("auth_user").(auth.User) }
func examQuery(c *gin.Context) (exams.Query, error) {
	page, size, err := parsePage(c)
	return exams.Query{SessionID: c.Query("session_id"), ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), Search: c.Query("search"), Page: page, PageSize: size}, err
}
func (h *examHandler) list(c *gin.Context) { h.listFor(c, "admin") }
func (h *examHandler) assigned(c *gin.Context) {
	if examUser(c).Role != "teacher" {
		failure(c, 403, "forbidden", "teacher account required")
		return
	}
	h.listFor(c, "teacher")
}
func (h *examHandler) listFor(c *gin.Context, scope string) {
	if !h.ready(c) {
		return
	}
	q, err := examQuery(c)
	if err != nil {
		examError(c, exams.ErrInvalid)
		return
	}
	v, err := h.service.ListExams(c.Request.Context(), q, scope, examUser(c).ID)
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) get(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.GetExam(c.Request.Context(), c.Param("id"))
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) assignedExam(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if examUser(c).Role != "teacher" {
		examError(c, exams.ErrForbidden)
		return
	}
	if _, err := h.service.Roster(c.Request.Context(), c.Param("id"), examUser(c).ID, "teacher"); err != nil {
		examError(c, err)
		return
	}
	h.get(c)
}
func (h *examHandler) save(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in exams.ExamInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.SaveExam(c.Request.Context(), examUser(c).ID, c.Param("id"), in)
	if err != nil {
		examError(c, err)
		return
	}
	status := 200
	if c.Param("id") == "" {
		status = http.StatusCreated
	}
	success(c, status, v)
}
func (h *examHandler) delete(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if err := h.service.DeleteExam(c.Request.Context(), examUser(c).ID, c.Param("id")); err != nil {
		examError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
func (h *examHandler) subject(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in exams.SubjectInput
	if !decodeSchoolJSON(c, 4*1024, &in) {
		return
	}
	v, err := h.service.SaveSubject(c.Request.Context(), examUser(c).ID, c.Param("id"), in)
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) deleteSubject(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if err := h.service.DeleteSubject(c.Request.Context(), examUser(c).ID, c.Param("id"), c.Param("subjectId")); err != nil {
		examError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
func (h *examHandler) roster(c *gin.Context) { h.rosterFor(c, "admin") }
func (h *examHandler) assignedRoster(c *gin.Context) {
	if examUser(c).Role != "teacher" {
		examError(c, exams.ErrForbidden)
		return
	}
	h.rosterFor(c, "teacher")
}
func (h *examHandler) rosterFor(c *gin.Context, scope string) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Roster(c.Request.Context(), c.Param("id"), examUser(c).ID, scope)
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) marks(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in exams.MarksInput
	if !decodeSchoolJSON(c, 128*1024, &in) {
		return
	}
	if err := h.service.SaveMarks(c.Request.Context(), examUser(c).ID, examUser(c).Role, c.Param("id"), in.Entries); err != nil {
		examError(c, err)
		return
	}
	success(c, 200, gin.H{"saved": len(in.Entries)})
}
func (h *examHandler) state(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decodeSchoolJSON(c, 1024, &in) {
		return
	}
	v, err := h.service.SetState(c.Request.Context(), examUser(c).ID, c.Param("id"), in.Status)
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) tabulation(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Tabulation(c.Request.Context(), c.Param("id"))
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) cards(c *gin.Context)      { h.cardsFor(c, "student", "") }
func (h *examHandler) childCards(c *gin.Context) { h.cardsFor(c, "guardian", c.Param("studentId")) }
func (h *examHandler) cardsFor(c *gin.Context, scope, studentID string) {
	if !h.ready(c) {
		return
	}
	if examUser(c).Role != scope {
		examError(c, exams.ErrForbidden)
		return
	}
	v, err := h.service.Cards(c.Request.Context(), scope, examUser(c).ID, studentID, "")
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *examHandler) card(c *gin.Context)      { h.cardFor(c, "student", "") }
func (h *examHandler) childCard(c *gin.Context) { h.cardFor(c, "guardian", c.Param("studentId")) }
func (h *examHandler) adminCard(c *gin.Context) { h.cardFor(c, "admin", c.Param("studentId")) }
func (h *examHandler) cardFor(c *gin.Context, scope, studentID string) {
	if !h.ready(c) {
		return
	}
	if scope != "admin" && examUser(c).Role != scope {
		examError(c, exams.ErrForbidden)
		return
	}
	id := c.Param("examId")
	if id == "" {
		id = c.Param("id")
	}
	v, err := h.service.Card(c.Request.Context(), scope, examUser(c).ID, studentID, id)
	if err != nil {
		examError(c, err)
		return
	}
	success(c, 200, v)
}
