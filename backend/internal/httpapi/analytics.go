package httpapi

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/analytics"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

type analyticsHandler struct {
	svc     *analytics.Service
	auth    *auth.Service
	modules *school.Service // nil only in isolated router tests
}

func (h *analyticsHandler) ready(c *gin.Context) bool {
	if h.svc != nil && h.auth != nil {
		return true
	}
	failure(c, 503, "analytics_unavailable", "analytics requires a database")
	return false
}

func analyticsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, analytics.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid analytics request")
	case errors.Is(err, analytics.ErrNotFound):
		failure(c, 404, "not_found", "not found")
	case errors.Is(err, analytics.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	default:
		_ = c.Error(err) // logged with the request; never sent to the client
		failure(c, 500, "internal_error", "internal server error")
	}
}

func (h *analyticsHandler) has(c *gin.Context, permission string) (bool, error) {
	return h.auth.HasPermission(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, permission)
}

// scope picks school scope for analytics:read and, when the route allows it,
// teacher scope for analytics:read_assigned.
func (h *analyticsHandler) scope(c *gin.Context, allowTeacher bool) (analytics.Scope, bool) {
	user := c.MustGet("auth_user").(auth.User)
	school, err := h.has(c, "analytics:read")
	if err != nil {
		analyticsError(c, err)
		return analytics.Scope{}, false
	}
	finance, err := h.has(c, "analytics:finance")
	if err != nil {
		analyticsError(c, err)
		return analytics.Scope{}, false
	}
	if school {
		return analytics.Scope{UserID: user.ID, Finance: finance}, true
	}
	if allowTeacher {
		assigned, err := h.has(c, "analytics:read_assigned")
		if err != nil {
			analyticsError(c, err)
			return analytics.Scope{}, false
		}
		if assigned {
			return analytics.Scope{Teacher: true, UserID: user.ID, Finance: finance}, true
		}
	}
	analyticsError(c, analytics.ErrForbidden)
	return analytics.Scope{}, false
}

func analyticsFilter(c *gin.Context) analytics.Filter {
	return analytics.Filter{SessionID: c.Query("session_id"), ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), ExamID: c.Query("exam_id"),
		ExamType: c.Query("exam_type"), From: c.Query("from"), To: c.Query("to"), Group: c.Query("group")}
}

func analyticsPage(c *gin.Context, size int) (int, int, bool) {
	page := 1
	var err error
	if raw := c.Query("page"); raw != "" {
		if page, err = strconv.Atoi(raw); err != nil {
			return 0, 0, false
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if size, err = strconv.Atoi(raw); err != nil {
			return 0, 0, false
		}
	}
	return page, size, page >= 1 && page <= 10000 && size >= 1 && size <= analytics.MaxPageSize
}

func (h *analyticsHandler) options(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	school, err := h.has(c, "analytics:read")
	if err != nil {
		analyticsError(c, err)
		return
	}
	assigned, err := h.has(c, "analytics:read_assigned")
	if err != nil {
		analyticsError(c, err)
		return
	}
	user := c.MustGet("auth_user").(auth.User)
	scope := analytics.Scope{UserID: user.ID, Teacher: !school && assigned}
	value, err := h.svc.Options(c.Request.Context(), scope, analyticsFilter(c), school || assigned)
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) overview(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	scope, ok := h.scope(c, false)
	if !ok {
		return
	}
	value, err := h.svc.Overview(c.Request.Context(), scope, c.Query("session_id"))
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) attendance(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	value, err := h.svc.Attendance(c.Request.Context(), scope, analyticsFilter(c))
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) exams(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	value, err := h.svc.Exams(c.Request.Context(), scope, analyticsFilter(c))
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) fees(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	value, err := h.svc.Fees(c.Request.Context(), analyticsFilter(c))
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) sessions(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	finance, err := h.has(c, "analytics:finance")
	if err != nil {
		analyticsError(c, err)
		return
	}
	value, err := h.svc.Sessions(c.Request.Context(), finance)
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, gin.H{"items": value})
}

func (h *analyticsHandler) risks(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	page, size, valid := analyticsPage(c, 25)
	if !valid {
		analyticsError(c, analytics.ErrInvalid)
		return
	}
	value, err := h.svc.Risks(c.Request.Context(), scope, analyticsFilter(c), c.Query("level"), c.Query("reason"), page, size)
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) settings(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	value, err := h.svc.Settings(c.Request.Context())
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) saveSettings(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var in analytics.Settings
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil || decoder.More() {
		analyticsError(c, analytics.ErrInvalid)
		return
	}
	value, err := h.svc.SaveSettings(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, in)
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) me(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	fees, err := h.has(c, "fees:read_self")
	if err != nil {
		analyticsError(c, err)
		return
	}
	value, err := h.svc.StudentSummary(c.Request.Context(), analytics.StudentAccess{Kind: "self", UserID: c.MustGet("auth_user").(auth.User).ID, Fees: fees}, "")
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) children(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	fees, err := h.has(c, "fees:read_children")
	if err != nil {
		analyticsError(c, err)
		return
	}
	value, err := h.svc.Children(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, fees)
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, gin.H{"items": value})
}

func (h *analyticsHandler) child(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	fees, err := h.has(c, "fees:read_children")
	if err != nil {
		analyticsError(c, err)
		return
	}
	value, err := h.svc.StudentSummary(c.Request.Context(), analytics.StudentAccess{Kind: "guardian", UserID: c.MustGet("auth_user").(auth.User).ID, Fees: fees}, c.Param("studentId"))
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

// student serves staff: analytics:read reads any student; a teacher reads
// only students of assigned classes. Fees appear for finance viewers.
func (h *analyticsHandler) student(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	access := analytics.StudentAccess{Kind: "school", UserID: scope.UserID, Fees: scope.Finance}
	if scope.Teacher {
		access.Kind = "teacher"
	}
	value, err := h.svc.StudentSummary(c.Request.Context(), access, c.Param("id"))
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *analyticsHandler) reportList(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	success(c, 200, gin.H{"items": analytics.Reports(), "max_rows": analytics.MaxReportRows})
}

// reportScope enforces a report's access rule and its source module.
func (h *analyticsHandler) reportScope(c *gin.Context) (analytics.Report, analytics.Scope, bool) {
	rep, found := analytics.ReportByKey(c.Param("report"))
	if !found {
		analyticsError(c, analytics.ErrNotFound)
		return rep, analytics.Scope{}, false
	}
	var scope analytics.Scope
	switch rep.Access {
	case "finance":
		finance, err := h.has(c, "analytics:finance")
		if err != nil {
			analyticsError(c, err)
			return rep, scope, false
		}
		if !finance {
			analyticsError(c, analytics.ErrForbidden)
			return rep, scope, false
		}
		scope = analytics.Scope{UserID: c.MustGet("auth_user").(auth.User).ID, Finance: true}
	default:
		var ok bool
		if scope, ok = h.scope(c, rep.Access == "academic"); !ok {
			return rep, scope, false
		}
	}
	if rep.Module != "" && h.modules != nil {
		enabled, err := h.modules.Enabled(c.Request.Context(), rep.Module)
		if err != nil {
			analyticsError(c, err)
			return rep, scope, false
		}
		if !enabled {
			schoolError(c, school.ErrModuleDisabled)
			return rep, scope, false
		}
	}
	return rep, scope, true
}

func (h *analyticsHandler) report(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	_, scope, ok := h.reportScope(c)
	if !ok {
		return
	}
	page, size, valid := analyticsPage(c, 50)
	if !valid {
		analyticsError(c, analytics.ErrInvalid)
		return
	}
	value, err := h.svc.Report(c.Request.Context(), scope, c.Param("report"), analyticsFilter(c), page, size)
	if err != nil {
		analyticsError(c, err)
		return
	}
	success(c, 200, value)
}

// reportCSV builds the whole report before writing, so a failure is still a
// JSON error rather than a truncated download.
func (h *analyticsHandler) reportCSV(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	_, scope, ok := h.reportScope(c)
	if !ok {
		return
	}
	value, err := h.svc.Report(c.Request.Context(), scope, c.Param("report"), analyticsFilter(c), 1, analytics.MaxReportRows)
	if err != nil {
		analyticsError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": analytics.FileName(value)}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Total-Rows", strconv.FormatInt(value.Total, 10))
	c.Status(http.StatusOK)
	if err := h.svc.WriteCSV(c.Request.Context(), c.Writer, scope.UserID, value); err != nil {
		_ = c.Error(err)
	}
}

func registerAnalytics(g *gin.RouterGroup, service *auth.Service, svc *analytics.Service, schoolService *school.Service, guard func(string) gin.HandlerFunc) {
	h := &analyticsHandler{svc: svc, auth: service, modules: schoolService}
	perm := func(p string) gin.HandlerFunc { return RequirePermission(service, p) }
	anyOf := func(p ...string) gin.HandlerFunc { return RequireAnyPermission(service, p...) }
	on := guard("analytics")
	academic := anyOf("analytics:read", "analytics:read_assigned")
	g.GET("/analytics/options", anyOf("analytics:read", "analytics:read_assigned", "analytics:finance"), on, h.options)
	g.GET("/analytics/overview", perm("analytics:read"), on, h.overview)
	g.GET("/analytics/attendance", academic, on, guard("attendance"), h.attendance)
	g.GET("/analytics/exams", academic, on, guard("examinations"), h.exams)
	g.GET("/analytics/fees", perm("analytics:finance"), on, guard("fees"), h.fees)
	g.GET("/analytics/sessions", perm("analytics:read"), on, h.sessions)
	g.GET("/analytics/risks", academic, on, h.risks)
	g.GET("/analytics/settings", academic, on, h.settings)
	g.PUT("/analytics/settings", perm("analytics:manage"), on, RequireAdministrator(), h.saveSettings)
	g.GET("/analytics/students/:id", academic, on, h.student)
	g.GET("/analytics/me", perm("analytics:read_self"), on, RequireAnyRole("student"), h.me)
	g.GET("/analytics/children", perm("analytics:read_children"), on, RequireAnyRole("guardian"), h.children)
	g.GET("/analytics/children/:studentId", perm("analytics:read_children"), on, RequireAnyRole("guardian"), h.child)
	g.GET("/analytics/reports", anyOf("analytics:read", "analytics:read_assigned", "analytics:finance"), on, h.reportList)
	g.GET("/analytics/reports/:report", anyOf("analytics:read", "analytics:read_assigned", "analytics:finance"), on, h.report)
	g.GET("/analytics/reports/:report/csv", anyOf("analytics:read", "analytics:read_assigned", "analytics:finance"), on, h.reportCSV)
}
