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
	var bulk *people.BulkError
	switch {
	case errors.As(err, &match):
		c.JSON(409, gin.H{"error": gin.H{"code": "guardian_exists", "message": "a guardian with this phone or email is already on record; link them or confirm a new guardian", "index": match.Index, "matches": match.Matches}})
	case errors.As(err, &bulk):
		c.JSON(422, gin.H{"error": gin.H{"code": "rows_have_errors", "message": "some rows have problems; nothing was admitted"}, "data": bulk.Preview})
	case errors.Is(err, people.ErrSheetFormat):
		failure(c, 400, "sheet_format", "the file is not a readable CSV or .xlsx spreadsheet")
	case errors.Is(err, people.ErrSheetNotUTF8):
		failure(c, 400, "sheet_not_utf8", "save the CSV as UTF-8 so Bangla text survives")
	case errors.Is(err, people.ErrSheetTooBig):
		failure(c, 400, "too_many_rows", "a file may hold at most 1000 students")
	case errors.Is(err, people.ErrBulkEmpty):
		failure(c, 400, "empty_sheet", "the file has no student rows")
	case errors.Is(err, people.ErrBulkKeyReused):
		failure(c, 409, "idempotency_key_reused", "this upload key already admitted a different file")
	case errors.Is(err, people.ErrHasLogin):
		failure(c, 409, "student_has_login", "this student already has a login")
	case errors.Is(err, people.ErrNoLoginID):
		failure(c, 409, "no_login_id", "the Student ID and admission number are both unusable as a sign-in ID (spaces, or already used by another login)")
	case errors.Is(err, people.ErrNoStudentsToLink):
		failure(c, 409, "no_students_without_login", "every active student in this section already has a login")
	case errors.Is(err, people.ErrLoginTaken):
		failure(c, 409, "login_email_taken", "this login email is already registered")
	case errors.Is(err, people.ErrAdmissionNoTaken):
		failure(c, 409, "admission_no_taken", "this admission number is already used")
	case errors.Is(err, people.ErrAdmissionKeyReused):
		failure(c, 409, "idempotency_key_reused", "this request key already admitted a different student")
	case errors.Is(err, people.ErrRollTaken):
		failure(c, 409, "roll_taken", "this roll number is already used in the section")
	case errors.Is(err, people.ErrPhotoUploadsOff):
		failure(c, 403, "photo_uploads_off", "the school has turned student photo uploads off")
	case errors.Is(err, people.ErrNoStudent):
		failure(c, 404, "no_student", "this account is not linked to a student")
	case errors.Is(err, people.ErrNotPending):
		failure(c, 409, "not_pending", "no photo is waiting for review")
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
	return people.Query{Search: c.Query("search"), Status: c.Query("status"), SessionID: c.Query("session_id"), ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), WithoutLogin: c.Query("without_login") == "true", Page: page, PageSize: size}, err
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
	status := 201
	if result.Replayed {
		status = 200
	}
	success(c, status, result)
}

// createStudentLogin creates and links a login for one student. The response
// carries the temporary password once; it is not stored or logged.
func (h *peopleHandler) createStudentLogin(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var v struct {
		Email string `json:"email"`
	}
	if !decodeSchoolJSON(c, 2*1024, &v) {
		return
	}
	login, err := h.service.CreateStudentLogin(c.Request.Context(), actorFrom(c), c.Param("id"), v.Email)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 201, login)
}

// createSectionLogins gives every active student in a section without a
// login one, and returns their slips.
func (h *peopleHandler) createSectionLogins(c *gin.Context) {
	if !h.available(c) {
		return
	}
	logins, err := h.service.CreateSectionLogins(c.Request.Context(), actorFrom(c), c.Query("section_id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 201, gin.H{"logins": logins})
}

// bulkFile reads an uploaded spreadsheet (CSV or .xlsx), at most 4 MB.
func bulkFile(c *gin.Context) ([]byte, bool) {
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	switch {
	case err != nil:
		peopleError(c, people.ErrSheetFormat)
		return nil, false
	case kind != "text/csv" && kind != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" && kind != "application/octet-stream":
		peopleError(c, people.ErrSheetFormat)
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4<<20+1))
	if err != nil || len(body) > 4<<20 || len(body) == 0 {
		peopleError(c, people.ErrSheetFormat)
		return nil, false
	}
	return body, true
}

// bulkTemplate downloads the bulk admission template (.xlsx, or CSV).
func (h *peopleHandler) bulkTemplate(c *gin.Context) {
	if c.Query("format") == "csv" {
		c.Header("Content-Disposition", `attachment; filename="student-admission-template.csv"`)
		c.Data(200, "text/csv; charset=utf-8", people.TemplateCSV())
		return
	}
	file, err := people.TemplateXLSX()
	if err != nil {
		peopleError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="student-admission-template.xlsx"`)
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file)
}

// previewBulk checks every row of an uploaded spreadsheet; nothing is written.
func (h *peopleHandler) previewBulk(c *gin.Context) {
	if !h.available(c) {
		return
	}
	body, ok := bulkFile(c)
	if !ok {
		return
	}
	v, err := h.service.PreviewBulk(c.Request.Context(), actorFrom(c), c.Query("session_id"), body)
	if errors.Is(err, people.ErrBulkMissingCols) {
		c.JSON(400, gin.H{"error": gin.H{"code": "missing_columns", "message": "required columns are missing"}, "data": v})
		return
	}
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}

// admitBulk admits every ready row in one transaction, or none.
func (h *peopleHandler) admitBulk(c *gin.Context) {
	if !h.available(c) {
		return
	}
	body, ok := bulkFile(c)
	if !ok {
		return
	}
	v, err := h.service.AdmitBulk(c.Request.Context(), actorFrom(c), c.Query("session_id"), c.Query("idempotency_key"), c.Query("create_logins") == "true", body)
	if errors.Is(err, people.ErrBulkMissingCols) {
		failure(c, 400, "missing_columns", "required columns are missing")
		return
	}
	if err != nil {
		peopleError(c, err)
		return
	}
	status := 201
	if v.Replayed {
		status = 200
	}
	success(c, status, v)
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
