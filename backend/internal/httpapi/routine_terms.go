package httpapi

import (
	"errors"
	"fmt"
	"mime"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
)

// Term exam routine planner (E8). The per-section exam routine endpoints
// above are unchanged.

type termRoutineHandler struct{ service *routine.TermService }

func (h *termRoutineHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "routine_unavailable", "routines require a database")
	return false
}

func termRoutineError(c *gin.Context, err error) {
	var issues *routine.IssuesError
	switch {
	case errors.As(err, &issues):
		message := "the change breaks a routine rule"
		if issues.Code == "routine_not_ready" {
			message = "the routine breaks a rule that must be fixed first"
		}
		if len(issues.Issues) > 0 {
			message = issues.Issues[0].Message
		}
		c.JSON(409, gin.H{"error": gin.H{"code": issues.Code, "message": message, "issues": issues.Issues}})
	case errors.Is(err, routine.ErrState):
		failure(c, 409, "routine_state", "the routine version's status does not allow this")
	case errors.Is(err, routine.ErrDocuments):
		failure(c, 503, "documents_unavailable", "routine documents need the document renderer")
	default:
		routineError(c, err)
	}
}

func (h *termRoutineHandler) answer(c *gin.Context, status int, v any, err error) {
	if err != nil {
		termRoutineError(c, err)
		return
	}
	success(c, status, v)
}

func actorID(c *gin.Context) string { return routineUser(c).ID }

// @Summary List exam rooms
// @Tags routine
// @Router /routines/rooms [get]
func (h *termRoutineHandler) rooms(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.ListRooms(c.Request.Context(), c.Query("all") == "true")
	h.answer(c, 200, v, err)
}

// @Summary Create or update an exam room
// @Tags routine
// @Router /routines/rooms [post]
// @Router /routines/rooms/{id} [patch]
func (h *termRoutineHandler) saveRoom(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.RoomInput
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.SaveRoom(c.Request.Context(), actorID(c), c.Param("id"), in)
	status := 200
	if c.Param("id") == "" {
		status = 201
	}
	h.answer(c, status, v, err)
}

// @Summary List exam terms with their routine versions
// @Tags routine
// @Router /routines/terms [get]
func (h *termRoutineHandler) terms(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.ListTerms(c.Request.Context(), c.Query("session_id"))
	h.answer(c, 200, v, err)
}

// @Summary Start a term's exam routine (version 1, with sittings and papers)
// @Tags routine
// @Router /routines/versions [post]
func (h *termRoutineHandler) createVersion(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.VersionInput
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.CreateVersion(c.Request.Context(), actorID(c), in)
	h.answer(c, 201, v, err)
}

// @Summary Read a routine version with its rule check and amendment changes
// @Tags routine
// @Router /routines/versions/{id} [get]
func (h *termRoutineHandler) version(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.GetVersion(c.Request.Context(), c.Param("id"))
	h.answer(c, 200, v, err)
}

// @Summary Change a draft routine's dates and instructions
// @Tags routine
// @Router /routines/versions/{id} [patch]
func (h *termRoutineHandler) updateVersion(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.VersionInput
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.UpdateVersion(c.Request.Context(), actorID(c), c.Param("id"), in)
	h.answer(c, 200, v, err)
}

// @Summary Delete a draft routine version
// @Tags routine
// @Router /routines/versions/{id} [delete]
func (h *termRoutineHandler) deleteVersion(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	err := h.service.DeleteVersion(c.Request.Context(), actorID(c), c.Param("id"))
	h.answer(c, 200, gin.H{"deleted": true}, err)
}

// @Summary Add or change a sitting
// @Tags routine
// @Router /routines/versions/{id}/sittings [post]
// @Router /routines/versions/{id}/sittings/{sittingId} [patch]
func (h *termRoutineHandler) saveSitting(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.SittingInput
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.SaveSitting(c.Request.Context(), actorID(c), c.Param("id"), c.Param("sittingId"), in)
	h.answer(c, 200, v, err)
}

// @Summary Delete an unused sitting
// @Tags routine
// @Router /routines/versions/{id}/sittings/{sittingId} [delete]
func (h *termRoutineHandler) deleteSitting(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.DeleteSitting(c.Request.Context(), actorID(c), c.Param("id"), c.Param("sittingId"))
	h.answer(c, 200, v, err)
}

// @Summary Add a paper (practical, viva, a second paper …)
// @Tags routine
// @Router /routines/versions/{id}/papers [post]
func (h *termRoutineHandler) addPaper(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.PaperInput
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.AddPaper(c.Request.Context(), actorID(c), c.Param("id"), in)
	h.answer(c, 200, v, err)
}

// @Summary Place a paper by hand (date, sitting, rooms); hard rules are checked
// @Tags routine
// @Router /routines/versions/{id}/papers/{paperId} [patch]
func (h *termRoutineHandler) placePaper(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.Placement
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.PlacePaper(c.Request.Context(), actorID(c), c.Param("id"), c.Param("paperId"), in)
	h.answer(c, 200, v, err)
}

// @Summary Remove a paper from a draft
// @Tags routine
// @Router /routines/versions/{id}/papers/{paperId} [delete]
func (h *termRoutineHandler) deletePaper(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.DeletePaper(c.Request.Context(), actorID(c), c.Param("id"), c.Param("paperId"))
	h.answer(c, 200, v, err)
}

// @Summary Add an invigilation duty
// @Tags routine
// @Router /routines/versions/{id}/duties [post]
func (h *termRoutineHandler) addDuty(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in routine.DutyInput
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.AddDuty(c.Request.Context(), actorID(c), c.Param("id"), in)
	h.answer(c, 200, v, err)
}

// @Summary Remove an invigilation duty
// @Tags routine
// @Router /routines/versions/{id}/duties/{dutyId} [delete]
func (h *termRoutineHandler) deleteDuty(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.DeleteDuty(c.Request.Context(), actorID(c), c.Param("id"), c.Param("dutyId"))
	h.answer(c, 200, v, err)
}

// @Summary Draft the routine with the deterministic generator
// @Tags routine
// @Router /routines/versions/{id}/generate [post]
func (h *termRoutineHandler) generate(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Generate(c.Request.Context(), actorID(c), c.Param("id"))
	h.answer(c, 200, v, err)
}

type reasonBody struct {
	Reason string `json:"reason"`
}

// move returns a handler that moves a version to a status.
func (h *termRoutineHandler) move(to string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.ready(c) {
			return
		}
		var in reasonBody
		if to == "draft" && c.ShouldBindJSON(&in) != nil {
			termRoutineError(c, routine.ErrInvalid)
			return
		}
		v, err := h.service.Move(c.Request.Context(), actorID(c), c.Param("id"), to, in.Reason)
		h.answer(c, 200, v, err)
	}
}

// @Summary Amend a published routine (a new draft version with a reason)
// @Tags routine
// @Router /routines/versions/{id}/amend [post]
func (h *termRoutineHandler) amend(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in reasonBody
	if c.ShouldBindJSON(&in) != nil {
		termRoutineError(c, routine.ErrInvalid)
		return
	}
	v, err := h.service.Amend(c.Request.Context(), actorID(c), c.Param("id"), in.Reason)
	h.answer(c, 201, v, err)
}

func sendPDF(c *gin.Context, name string, pdf []byte, err error) {
	if err != nil {
		termRoutineError(c, err)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "application/pdf", pdf)
}

// @Summary Routine PDF: class routines, room schedules or invigilation duties
// @Tags routine
// @Router /routines/versions/{id}/pdf [get]
func (h *termRoutineHandler) pdf(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	in := routine.DocumentRequest{Kind: c.DefaultQuery("kind", "class"), Language: c.Query("lang"), ClassID: c.Query("class_id"), RoomID: c.Query("room_id")}
	pdf, err := h.service.Document(c.Request.Context(), c.Param("id"), in)
	lang := in.Language
	if lang == "" {
		lang = "en"
	}
	sendPDF(c, fmt.Sprintf("exam-routine-%s-%s.pdf", in.Kind, lang), pdf, err)
}

// @Summary The signed-in student's published term exam routines
// @Tags routine
// @Router /routines/terms/me [get]
func (h *termRoutineHandler) mine(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if routineUser(c).Role != "student" {
		failure(c, 403, "forbidden", "own exam routine is for students")
		return
	}
	v, err := h.service.StudentRoutines(c.Request.Context(), actorID(c))
	h.answer(c, 200, v, err)
}

// @Summary The signed-in student's routine PDF
// @Tags routine
// @Router /routines/terms/me/{versionId}/pdf [get]
func (h *termRoutineHandler) minePDF(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if routineUser(c).Role != "student" {
		failure(c, 403, "forbidden", "own exam routine is for students")
		return
	}
	lang := c.DefaultQuery("lang", "en")
	pdf, err := h.service.StudentDocument(c.Request.Context(), actorID(c), c.Param("versionId"), lang)
	sendPDF(c, "my-exam-routine-"+lang+".pdf", pdf, err)
}

// @Summary The signed-in teacher's invigilation duties
// @Tags routine
// @Router /routines/duties/me [get]
func (h *termRoutineHandler) duties(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if routineUser(c).Role != "teacher" {
		failure(c, 403, "forbidden", "duties are for teachers")
		return
	}
	v, err := h.service.TeacherDuties(c.Request.Context(), actorID(c))
	h.answer(c, 200, v, err)
}
