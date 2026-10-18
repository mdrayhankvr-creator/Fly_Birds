package httpapi

import (
	"context"
	"errors"
	"strings"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
)

type auditHandler struct{ service *audit.Service }

// events godoc
// @Summary Read filtered audit events; requires audit:read
// @Tags audit
// @Router /audit/events [get]
func (h *auditHandler) events(c *gin.Context) {
	if h.service == nil {
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
	value, err := h.service.List(c.Request.Context(), audit.Query{Actor: c.Query("actor"), Action: c.Query("action"), TargetType: c.Query("target_type"), TargetID: c.Query("target_id"), Outcome: c.Query("outcome"), From: c.Query("from"), To: c.Query("to"), Search: c.Query("search"), Page: page, PageSize: size, Newest: sort == "newest"})
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

type dashboardHandler struct{ service *dashboard.Service }

// dashboard godoc
// @Summary Get current role dashboard data
// @Tags dashboard
// @Router /dashboard [get]
func (h *dashboardHandler) get(c *gin.Context) {
	if h.service == nil {
		failure(c, 503, "dashboard_unavailable", "dashboard requires a database")
		return
	}
	user := c.MustGet("auth_user").(auth.User)
	value, err := h.service.Get(c.Request.Context(), user)
	if errors.Is(err, dashboard.ErrForbidden) {
		failure(c, 403, "forbidden", "permission denied")
		return
	}
	if err != nil {
		failure(c, 500, "internal_error", "internal server error")
		return
	}
	success(c, http.StatusOK, value)
}

// actions godoc
// @Summary List recorded audit action names with counts
// @Tags audit
// @Router /audit/actions [get]
func (h *auditHandler) actions(c *gin.Context) {
	if h.service == nil {
		failure(c, 503, "audit_unavailable", "audit log requires a database")
		return
	}
	value, err := h.service.Actions(c.Request.Context())
	if err != nil {
		failure(c, 500, "internal_error", "internal server error")
		return
	}
	success(c, http.StatusOK, value)
}

// sensitiveRoutes are administrative endpoints whose refusals are audited.
var sensitiveRoutes = []string{"/admin/", "/audit/", "/users", "/roles", "/permissions", "/school/settings", "/school/grading", "/school/modules"}

// AuditDenied records 401/403 responses from sensitive administrative routes
// so attempted misuse is visible in the audit log. It runs before the guards
// and inspects the final status after them.
func AuditDenied(service *audit.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		status := c.Writer.Status()
		if service == nil || (status != http.StatusForbidden && status != http.StatusUnauthorized) {
			return
		}
		route := c.FullPath()
		sensitive := false
		for _, prefix := range sensitiveRoutes {
			sensitive = sensitive || strings.HasPrefix(route, prefix)
		}
		if !sensitive {
			return
		}
		var actor *string
		if value, ok := c.Get("auth_user"); ok {
			if user, valid := value.(auth.User); valid {
				actor = &user.ID
			}
		}
		if actor == nil {
			return // unauthenticated probes are not attributable; keep the log meaningful
		}
		reason := "forbidden"
		if status == http.StatusUnauthorized {
			reason = "unauthorized"
		}
		service.Denied(context.WithoutCancel(c.Request.Context()), actor, c.Request.Method, route, reason)
	}
}
