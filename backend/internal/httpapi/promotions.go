package httpapi

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/promotion"
)

type promotionHandler struct{ service *promotion.Service }

func (h *promotionHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "promotions_unavailable", "promotion requires a database")
	return false
}

func promotionActor(c *gin.Context) promotion.Actor {
	u := c.MustGet("auth_user").(auth.User)
	return promotion.Actor{ID: u.ID, Role: u.Role}
}

func promotionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, promotion.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid promotion input")
	case errors.Is(err, promotion.ErrReasonRequired):
		failure(c, 400, "reason_required", "a reason is required for this decision")
	case errors.Is(err, promotion.ErrNotFound):
		failure(c, 404, "not_found", "record not found")
	case errors.Is(err, promotion.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	case errors.Is(err, promotion.ErrProgressionUnset):
		failure(c, 409, "progression_unset", "set the class's next class or mark it as the final class before promoting it")
	case errors.Is(err, promotion.ErrOpenBatch):
		failure(c, 409, "batch_open", "this class already has an open promotion batch for that session")
	case errors.Is(err, promotion.ErrState):
		failure(c, 409, "batch_state", "the batch's status does not allow this")
	case errors.Is(err, promotion.ErrBlocked):
		failure(c, 409, "batch_blocked", "the batch has blockers; resolve them first")
	case errors.Is(err, promotion.ErrStale):
		failure(c, 409, "inputs_changed", "the batch changed since it was reviewed or approved")
	case errors.Is(err, promotion.ErrRevertRefused):
		failure(c, 409, "revert_refused", "the new year already has records for these students, or a student has moved since")
	case errors.Is(err, promotion.ErrConflict):
		failure(c, 409, "promotion_conflict", "a student changed while applying, or a roll number is taken")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func (h *promotionHandler) answer(c *gin.Context, status int, v any, err error) {
	if err != nil {
		promotionError(c, err)
		return
	}
	success(c, status, v)
}

func (h *promotionHandler) list(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.List(c.Request.Context(), promotion.Query{SessionID: c.Query("session_id"), ClassID: c.Query("class_id")})
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) get(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Get(c.Request.Context(), c.Param("id"))
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) create(c *gin.Context) {
	var in promotion.CreateInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.Create(c.Request.Context(), promotionActor(c), in)
		h.answer(c, 201, v, err)
	}
}

func (h *promotionHandler) updateItem(c *gin.Context) {
	var in promotion.ItemInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.UpdateItem(c.Request.Context(), promotionActor(c), c.Param("id"), c.Param("studentId"), in)
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) arrange(c *gin.Context) {
	var in promotion.ArrangeInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.Arrange(c.Request.Context(), promotionActor(c), c.Param("id"), in)
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) refresh(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Refresh(c.Request.Context(), promotionActor(c), c.Param("id"))
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) discard(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Discard(c.Request.Context(), promotionActor(c), c.Param("id"))
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) approve(c *gin.Context) {
	var in promotion.HashInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.Approve(c.Request.Context(), promotionActor(c), c.Param("id"), in)
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) apply(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Apply(c.Request.Context(), promotionActor(c), c.Param("id"))
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) revert(c *gin.Context) {
	var in promotion.RevertInput
	if h.ready(c) && decodeSchoolJSON(c, 2*1024, &in) {
		v, err := h.service.Revert(c.Request.Context(), promotionActor(c), c.Param("id"), in)
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) progression(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Progression(c.Request.Context())
		h.answer(c, 200, v, err)
	}
}

func (h *promotionHandler) setProgression(c *gin.Context) {
	var in promotion.ProgressionInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.SetProgression(c.Request.Context(), promotionActor(c), c.Param("classId"), in)
		h.answer(c, 200, v, err)
	}
}

// registerPromotions adds the M15 year-end promotion routes. Everything
// needs the students module. The administrator prepares and applies; only
// the head teacher approves; students and guardians have none of these
// permissions.
func registerPromotions(router *gin.Engine, service *auth.Service, h *promotionHandler, studentsModule gin.HandlerFunc) {
	g := router.Group("/promotions")
	g.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service), studentsModule)
	read := RequirePermission(service, "promotions:read")
	manage := []gin.HandlerFunc{RequirePermission(service, "promotions:manage"), RequireAdministrator()}
	applyPerm := []gin.HandlerFunc{RequirePermission(service, "promotions:apply"), RequireAdministrator()}
	with := func(chain []gin.HandlerFunc, last gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, chain...), last)
	}
	g.GET("/progression", read, h.progression)
	g.PUT("/progression/:classId", with(manage, h.setProgression)...)
	g.GET("", read, h.list)
	g.POST("", with(manage, h.create)...)
	g.GET("/:id", read, h.get)
	g.PATCH("/:id/items/:studentId", with(manage, h.updateItem)...)
	g.POST("/:id/arrange", with(manage, h.arrange)...)
	g.POST("/:id/refresh", with(manage, h.refresh)...)
	g.POST("/:id/discard", with(manage, h.discard)...)
	g.POST("/:id/approve", RequirePermission(service, "promotions:approve"), h.approve)
	g.POST("/:id/apply", with(applyPerm, h.apply)...)
	g.POST("/:id/revert", with(applyPerm, h.revert)...)
}
