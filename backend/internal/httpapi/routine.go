package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
)

type routineHandler struct{ service *routine.Service }

func (h *routineHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "routine_unavailable", "routines require a database")
	return false
}
func routineError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, routine.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid routine input")
	case errors.Is(err, routine.ErrNotFound):
		failure(c, 404, "not_found", "routine not found")
	case errors.Is(err, routine.ErrConflict):
		failure(c, 409, "schedule_conflict", "routine conflicts with an existing schedule or published state")
	case errors.Is(err, routine.ErrForbidden):
		failure(c, 403, "forbidden", "routine access denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func routineUser(c *gin.Context) auth.User { return c.MustGet("auth_user").(auth.User) }
func routineQuery(c *gin.Context) (routine.Query, error) {
	page, size, err := parsePage(c)
	return routine.Query{SessionID: c.Query("session_id"), ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), Search: c.Query("search"), Page: page, PageSize: size}, err
}

// @Summary List class timetable periods with filtering and pagination
// @Tags routine
// @Router /routines/classes [get]
func (h *routineHandler) classes(c *gin.Context) { h.listClasses(c, "admin", "") }

// @Summary Read own weekly class or teaching timetable
// @Tags routine
// @Router /routines/classes/me [get]
func (h *routineHandler) ownClasses(c *gin.Context) {
	role := routineUser(c).Role
	if role != "student" && role != "teacher" {
		failure(c, 403, "forbidden", "own routine is for students and teachers")
		return
	}
	h.listClasses(c, role, "")
}

// @Summary Read a linked child's class timetable
// @Tags routine
// @Router /routines/classes/children/{studentId} [get]
func (h *routineHandler) childClasses(c *gin.Context) {
	h.listClasses(c, "guardian", c.Param("studentId"))
}
func (h *routineHandler) listClasses(c *gin.Context, scope, studentID string) {
	if !h.available(c) {
		return
	}
	q, err := routineQuery(c)
	if err != nil {
		routineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.ListClasses(c.Request.Context(), q, scope, routineUser(c).ID, studentID)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

// @Summary Create a weekly class timetable period
// @Tags routine
// @Router /routines/classes [post]
func (h *routineHandler) createClass(c *gin.Context) { h.saveClass(c, "") }

// @Summary Update a weekly class timetable period
// @Tags routine
// @Router /routines/classes/{id} [patch]
func (h *routineHandler) updateClass(c *gin.Context) { h.saveClass(c, c.Param("id")) }
func (h *routineHandler) saveClass(c *gin.Context, id string) {
	if !h.available(c) {
		return
	}
	var in routine.ClassInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.SaveClass(c.Request.Context(), routineUser(c).ID, id, in)
	if err != nil {
		routineError(c, err)
		return
	}
	status := 200
	if id == "" {
		status = http.StatusCreated
	}
	success(c, status, v)
}

// @Summary Delete a weekly class timetable period
// @Tags routine
// @Router /routines/classes/{id} [delete]
func (h *routineHandler) deleteClass(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteClass(c.Request.Context(), routineUser(c).ID, c.Param("id")); err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}

// @Summary List exam routines with filtering and pagination
// @Tags routine
// @Router /routines/exams [get]
func (h *routineHandler) exams(c *gin.Context) { h.listExams(c, "admin", "") }

// @Summary List published exam routines for the signed-in student
// @Tags routine
// @Router /routines/exams/me [get]
func (h *routineHandler) ownExams(c *gin.Context) {
	if routineUser(c).Role != "student" {
		failure(c, 403, "forbidden", "student account required")
		return
	}
	h.listExams(c, "student", "")
}

// @Summary List published exam routines for a linked child
// @Tags routine
// @Router /routines/exams/children/{studentId} [get]
func (h *routineHandler) childExams(c *gin.Context) { h.listExams(c, "guardian", c.Param("studentId")) }
func (h *routineHandler) listExams(c *gin.Context, scope, studentID string) {
	if !h.available(c) {
		return
	}
	q, err := routineQuery(c)
	if err != nil {
		routineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.ListExams(c.Request.Context(), q, scope, routineUser(c).ID, studentID)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

// @Summary Read an exam routine and its timetable entries
// @Tags routine
// @Router /routines/exams/{id} [get]
func (h *routineHandler) exam(c *gin.Context) { h.getExam(c, "admin", "", c.Param("id")) }

// @Summary Read one published exam routine for the signed-in student
// @Tags routine
// @Router /routines/exams/me/{id} [get]
func (h *routineHandler) ownExam(c *gin.Context) {
	if routineUser(c).Role != "student" {
		failure(c, 403, "forbidden", "student account required")
		return
	}
	h.getExam(c, "student", "", c.Param("id"))
}

// @Summary Read one published exam routine for a linked child
// @Tags routine
// @Router /routines/exams/children/{studentId}/{id} [get]
func (h *routineHandler) childExam(c *gin.Context) {
	h.getExam(c, "guardian", c.Param("studentId"), c.Param("id"))
}
func (h *routineHandler) getExam(c *gin.Context, scope, studentID, id string) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GetExam(c.Request.Context(), id, scope, routineUser(c).ID, studentID)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

// @Summary Create a draft exam routine
// @Tags routine
// @Router /routines/exams [post]
func (h *routineHandler) createExam(c *gin.Context) { h.saveExam(c, "") }

// @Summary Update a draft exam routine
// @Tags routine
// @Router /routines/exams/{id} [patch]
func (h *routineHandler) updateExam(c *gin.Context) { h.saveExam(c, c.Param("id")) }
func (h *routineHandler) saveExam(c *gin.Context, id string) {
	if !h.available(c) {
		return
	}
	var in routine.ExamInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.SaveExam(c.Request.Context(), routineUser(c).ID, id, in)
	if err != nil {
		routineError(c, err)
		return
	}
	status := 200
	if id == "" {
		status = 201
	}
	success(c, status, v)
}

// @Summary Publish or unpublish an exam routine
// @Tags routine
// @Router /routines/exams/{id}/publication [patch]
func (h *routineHandler) publication(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in struct {
		Published *bool `json:"published"`
	}
	if !decodeSchoolJSON(c, 1024, &in) {
		return
	}
	if in.Published == nil {
		routineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.PublishExam(c.Request.Context(), routineUser(c).ID, c.Param("id"), *in.Published)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

// @Summary Delete a draft exam routine
// @Tags routine
// @Router /routines/exams/{id} [delete]
func (h *routineHandler) deleteExam(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteExam(c.Request.Context(), routineUser(c).ID, c.Param("id")); err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}

// @Summary Add an exam timetable entry to a draft
// @Tags routine
// @Router /routines/exams/{id}/slots [post]
func (h *routineHandler) createExamSlot(c *gin.Context) { h.saveExamSlot(c, "") }

// @Summary Update an exam timetable entry
// @Tags routine
// @Router /routines/exams/{id}/slots/{slotId} [patch]
func (h *routineHandler) updateExamSlot(c *gin.Context) { h.saveExamSlot(c, c.Param("slotId")) }
func (h *routineHandler) saveExamSlot(c *gin.Context, id string) {
	if !h.available(c) {
		return
	}
	var in routine.ExamSlotInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.SaveExamSlot(c.Request.Context(), routineUser(c).ID, c.Param("id"), id, in)
	if err != nil {
		routineError(c, err)
		return
	}
	status := 200
	if id == "" {
		status = 201
	}
	success(c, status, v)
}

// @Summary Delete an exam timetable entry from a draft
// @Tags routine
// @Router /routines/exams/{id}/slots/{slotId} [delete]
func (h *routineHandler) deleteExamSlot(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteExamSlot(c.Request.Context(), routineUser(c).ID, c.Param("id"), c.Param("slotId")); err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
