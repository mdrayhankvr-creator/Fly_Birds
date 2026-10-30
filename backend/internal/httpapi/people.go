package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
)

type peopleHandler struct{ service *people.Service }

func (h *peopleHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "people_unavailable", "student records require a database")
	return false
}
func peopleError(c *gin.Context, err error) {
	var match *people.GuardianMatchError
	switch {
	case errors.As(err, &match):
		c.JSON(409, gin.H{"error": gin.H{"code": "guardian_exists", "message": "a guardian with this phone or email is already on record; link them or confirm a new guardian", "index": match.Index, "matches": match.Matches}})
	case errors.Is(err, people.ErrLoginTaken):
		failure(c, 409, "login_email_taken", "this login email is already registered")
	case errors.Is(err, people.ErrAdmissionNoTaken):
		failure(c, 409, "admission_no_taken", "this admission number is already used")
	case errors.Is(err, people.ErrRollTaken):
		failure(c, 409, "roll_taken", "this roll number is already used in the section")
	case errors.Is(err, people.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid student or guardian input")
	case errors.Is(err, people.ErrNotFound):
		failure(c, 404, "not_found", "record not found")
	case errors.Is(err, people.ErrDuplicate):
		failure(c, 409, "duplicate_record", "record already exists")
	case errors.Is(err, people.ErrUsePromotion):
		failure(c, 409, "use_promotion", "the student has records this year; move them to another session with year-end promotion")
	case errors.Is(err, people.ErrHistory):
		failure(c, 409, "enrolment_history", "this change would rewrite the student's enrolment history; use a section transfer or year-end promotion")
	case errors.Is(err, people.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func actorFrom(c *gin.Context) people.Actor {
	u := c.MustGet("auth_user").(auth.User)
	return people.Actor{ID: u.ID, Role: u.Role}
}
func peopleQuery(c *gin.Context) (people.Query, error) {
	page, size, err := parsePage(c)
	return people.Query{Search: c.Query("search"), Status: c.Query("status"), SessionID: c.Query("session_id"), ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), Page: page, PageSize: size}, err
}
func (h *peopleHandler) students(c *gin.Context) {
	if !h.available(c) {
		return
	}
	q, err := peopleQuery(c)
	if err != nil {
		peopleError(c, people.ErrInvalid)
		return
	}
	v, err := h.service.ListStudents(c.Request.Context(), q)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) student(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GetStudent(c.Request.Context(), actorFrom(c), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) enrolments(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Enrolments(c.Request.Context(), actorFrom(c), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) myStudent(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.OwnStudent(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) saveStudent(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var v people.StudentInput
	if !decodeSchoolJSON(c, 16*1024, &v) {
		return
	}
	result, err := h.service.SaveStudent(c.Request.Context(), actorFrom(c), c.Param("id"), v)
	if err != nil {
		peopleError(c, err)
		return
	}
	status := 200
	if c.Request.Method == http.MethodPost {
		status = 201
	}
	success(c, status, result)
}
func (h *peopleHandler) deleteStudent(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteStudent(c.Request.Context(), actorFrom(c), c.Param("id")); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
func (h *peopleHandler) importStudents(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, _, mediaErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaErr != nil || kind != "text/csv" {
		peopleError(c, people.ErrInvalid)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2097153))
	if err != nil || len(body) > 2097152 {
		peopleError(c, people.ErrInvalid)
		return
	}
	count, err := h.service.ImportCSV(c.Request.Context(), actorFrom(c), c.Query("session_id"), c.Query("class_id"), c.Query("section_id"), body)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 201, gin.H{"imported": count})
}
func (h *peopleHandler) savePhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		peopleError(c, people.ErrInvalid)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2097153))
	if err != nil {
		peopleError(c, people.ErrInvalid)
		return
	}
	if err := h.service.SavePhoto(c.Request.Context(), actorFrom(c), c.Param("id"), people.Photo{ContentType: kind, Content: body}); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"saved": true})
}
func (h *peopleHandler) photo(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GetPhoto(c.Request.Context(), actorFrom(c), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, v.ContentType, v.Content)
}
func (h *peopleHandler) guardians(c *gin.Context) {
	if !h.available(c) {
		return
	}
	q, err := peopleQuery(c)
	if err != nil {
		peopleError(c, people.ErrInvalid)
		return
	}
	v, err := h.service.ListGuardians(c.Request.Context(), q)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) guardian(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GetGuardian(c.Request.Context(), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) myGuardian(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.OwnGuardian(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) saveGuardian(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var v people.GuardianInput
	if !decodeSchoolJSON(c, 16*1024, &v) {
		return
	}
	result, err := h.service.SaveGuardian(c.Request.Context(), actorFrom(c), c.Param("id"), v)
	if err != nil {
		peopleError(c, err)
		return
	}
	status := 200
	if c.Request.Method == http.MethodPost {
		status = 201
	}
	success(c, status, result)
}
func (h *peopleHandler) deleteGuardian(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteGuardian(c.Request.Context(), actorFrom(c), c.Param("id")); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
func (h *peopleHandler) relationships(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.ListRelationships(c.Request.Context(), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) myGuardians(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.OwnGuardians(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) link(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var v people.LinkInput
	if !decodeSchoolJSON(c, 8*1024, &v) {
		return
	}
	result, err := h.service.Link(c.Request.Context(), actorFrom(c), c.Param("id"), v)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, result)
}
func (h *peopleHandler) unlink(c *gin.Context) {
	if !h.available(c) {
		return
	}
	err := h.service.Unlink(c.Request.Context(), actorFrom(c), c.Param("id"), c.Param("guardianId"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
func (h *peopleHandler) children(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Children(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) guardianStudents(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GuardianStudents(c.Request.Context(), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) child(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GetStudent(c.Request.Context(), actorFrom(c), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}

// admit creates a student, an optional login and up to two guardians in one
// transaction. The response carries the login's initial password once.
func (h *peopleHandler) admit(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var v people.AdmissionInput
	if !decodeSchoolJSON(c, 32*1024, &v) {
		return
	}
	result, err := h.service.Admit(c.Request.Context(), actorFrom(c), v)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 201, result)
}
func (h *peopleHandler) admissionDefaults(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.AdmissionDefaults(c.Request.Context(), actorFrom(c), c.Query("section_id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *peopleHandler) guardianMatches(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GuardianMatches(c.Request.Context(), c.Query("phone"), c.Query("email"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}
