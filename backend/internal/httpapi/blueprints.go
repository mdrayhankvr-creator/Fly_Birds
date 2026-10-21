package httpapi

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
)

// blueprintHandler serves exam terms, assessment blueprints and optional
// subject choices (E5). Reading needs exams:read; changing needs exams:manage.
type blueprintHandler struct{ service *blueprints.Service }

func (h *blueprintHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "exams_unavailable", "exams require a database")
	return false
}

func blueprintError(c *gin.Context, err error) {
	var invalid *blueprints.ValidationError
	switch {
	case errors.As(err, &invalid):
		c.JSON(422, gin.H{"error": gin.H{"code": "blueprint_invalid", "message": "the blueprint has problems to fix before it can be activated", "issues": invalid.Issues}})
	case errors.Is(err, blueprints.ErrInvalidInput), errors.Is(err, blueprints.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid blueprint input")
	case errors.Is(err, blueprints.ErrNotFound):
		failure(c, 404, "not_found", "term, blueprint or version not found")
	case errors.Is(err, blueprints.ErrInUse):
		failure(c, 409, "in_use", "it is already used by exams or activated, so it cannot change")
	case errors.Is(err, blueprints.ErrConflict):
		failure(c, 409, "blueprint_conflict", "a term with that name or order, or an open draft, already exists")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func (h *blueprintHandler) respond(c *gin.Context, status int, v any, err error) {
	if err != nil {
		blueprintError(c, err)
		return
	}
	success(c, status, v)
}

func (h *blueprintHandler) terms(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Terms(c.Request.Context(), c.Query("session_id"))
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) saveTerm(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in blueprints.TermInput
	if !decodeSchoolJSON(c, 4*1024, &in) {
		return
	}
	v, err := h.service.SaveTerm(c.Request.Context(), examUser(c).ID, c.Param("id"), in)
	status := 200
	if c.Param("id") == "" {
		status = 201
	}
	h.respond(c, status, v, err)
}

func (h *blueprintHandler) deleteTerm(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if err := h.service.DeleteTerm(c.Request.Context(), examUser(c).ID, c.Param("id")); err != nil {
		blueprintError(c, err)
		return
	}
	c.Status(204)
}

func (h *blueprintHandler) classBlueprints(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.ClassBlueprints(c.Request.Context(), c.Query("session_id"))
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) createBlueprint(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in struct {
		SessionID string `json:"session_id"`
		ClassID   string `json:"class_id"`
		CopyFrom  string `json:"copy_from"`
	}
	if !decodeSchoolJSON(c, 4*1024, &in) {
		return
	}
	v, err := h.service.CreateBlueprint(c.Request.Context(), examUser(c).ID, in.SessionID, in.ClassID, in.CopyFrom)
	h.respond(c, 201, v, err)
}

func (h *blueprintHandler) newVersion(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in struct {
		FromVersionID string `json:"from_version_id"`
	}
	if !decodeSchoolJSON(c, 4*1024, &in) {
		return
	}
	v, err := h.service.NewDraft(c.Request.Context(), examUser(c).ID, c.Param("id"), in.FromVersionID)
	h.respond(c, 201, v, err)
}

func (h *blueprintHandler) version(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Version(c.Request.Context(), c.Param("id"))
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) saveDraft(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in struct {
		Definition blueprints.Definition `json:"definition"`
	}
	if !decodeSchoolJSON(c, 512*1024, &in) {
		return
	}
	v, err := h.service.SaveDraft(c.Request.Context(), examUser(c).ID, c.Param("id"), in.Definition)
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) deleteDraft(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if err := h.service.DeleteDraft(c.Request.Context(), examUser(c).ID, c.Param("id")); err != nil {
		blueprintError(c, err)
		return
	}
	c.Status(204)
}

func (h *blueprintHandler) activate(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Activate(c.Request.Context(), examUser(c).ID, c.Param("id"))
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) visibility(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in struct {
		Key        string `json:"key"`
		Visibility string `json:"visibility"`
	}
	if !decodeSchoolJSON(c, 4*1024, &in) {
		return
	}
	v, err := h.service.SetVisibility(c.Request.Context(), examUser(c).ID, c.Param("id"), in.Key, in.Visibility)
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) sample(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in blueprints.SampleRequest
	if !decodeSchoolJSON(c, 512*1024, &in) {
		return
	}
	v, err := h.service.Sample(c.Request.Context(), c.Param("id"), in)
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) choices(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.Choices(c.Request.Context(), c.Query("session_id"), c.Query("class_id"))
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) saveChoices(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in struct {
		SessionID string              `json:"session_id"`
		ClassID   string              `json:"class_id"`
		Choices   []blueprints.Choice `json:"choices"`
	}
	if !decodeSchoolJSON(c, 128*1024, &in) {
		return
	}
	v, err := h.service.SaveChoices(c.Request.Context(), examUser(c).ID, in.SessionID, in.ClassID, in.Choices)
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) setup(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.SetupPlan(c.Request.Context(), c.Param("id"))
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) saveSetup(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in blueprints.Setup
	if !decodeSchoolJSON(c, 128*1024, &in) {
		return
	}
	v, err := h.service.SaveSetup(c.Request.Context(), examUser(c).ID, c.Param("id"), in)
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) createTerm(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.CreateTerm(c.Request.Context(), examUser(c).ID, c.Param("id"))
	if errors.Is(err, blueprints.ErrSetupBlocked) {
		failure(c, 409, "setup_blocked", "choose at least one ready class, give every assessment a due date and resolve name conflicts first")
		return
	}
	h.respond(c, 200, v, err)
}

func (h *blueprintHandler) termPreview(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	v, err := h.service.TermPreview(c.Request.Context(), c.Param("id"), c.Query("class_id"))
	h.respond(c, 200, v, err)
}
