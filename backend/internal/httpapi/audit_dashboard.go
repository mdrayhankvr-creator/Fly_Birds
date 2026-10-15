package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
)

type auditHandler struct{ repo *audit.Repository }

// events godoc
// @Summary Read filtered audit events; requires audit:read
// @Tags audit
// @Router /audit/events [get]
func (h *auditHandler) events(c *gin.Context) {
	if h.repo == nil {
		failure(c, 503, "audit_unavailable", "audit log requires a database")
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		failure(c, 400, "invalid_request", "invalid pagination")
		return
	}
	sort := c.DefaultQuery("sort", "newest")
	if sort != "newest" && sort != "oldest" {
		failure(c, 400, "invalid_request", "invalid sort order")
		return
	}
	value, err := h.repo.List(c.Request.Context(), audit.Query{Actor: c.Query("actor"), Action: c.Query("action"), TargetType: c.Query("target_type"), Page: page, PageSize: size, Newest: sort == "newest"})
	if errors.Is(err, audit.ErrInvalid) {
		failure(c, 400, "invalid_request", "invalid audit filter")
		return
	}
	if err != nil {
		failure(c, 500, "internal_error", "internal server error")
		return
	}
	success(c, http.StatusOK, value)
}

type dashboardHandler struct{ repo *dashboard.Repository }

// dashboard godoc
// @Summary Get current role dashboard data
// @Tags dashboard
// @Router /dashboard [get]
func (h *dashboardHandler) get(c *gin.Context) {
	if h.repo == nil {
		failure(c, 503, "dashboard_unavailable", "dashboard requires a database")
		return
	}
	user := c.MustGet("auth_user").(auth.User)
	value, err := h.repo.Get(c.Request.Context(), user)
	if err != nil {
		failure(c, 500, "internal_error", "internal server error")
		return
	}
	success(c, http.StatusOK, value)
}
