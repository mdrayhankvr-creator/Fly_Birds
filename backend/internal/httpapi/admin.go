package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

// AdminServices groups the M10 control-center services.
type AdminServices struct {
	Exporter *admin.Exporter
	Backups  *admin.Backups
	Overview *admin.OverviewReader
}

type adminHandler struct{ svc *AdminServices }

func (h *adminHandler) ready(c *gin.Context) bool {
	if h.svc != nil && h.svc.Exporter != nil && h.svc.Backups != nil && h.svc.Overview != nil {
		return true
	}
	failure(c, 503, "admin_unavailable", "administration requires a database")
	return false
}

func adminError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, admin.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid administration request")
	case errors.Is(err, admin.ErrNotFound):
		failure(c, 404, "not_found", "not found")
	case errors.Is(err, admin.ErrBusy):
		failure(c, 409, "backup_running", "another backup is already running")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func adminUser(c *gin.Context) auth.User { return c.MustGet("auth_user").(auth.User) }

// overview godoc
// @Summary Read the administrator control center overview
// @Tags admin
// @Router /admin/overview [get]
func (h *adminHandler) overview(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	value, err := h.svc.Overview.Overview(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *adminHandler) datasets(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	value, err := h.svc.Exporter.Datasets(c.Request.Context(), c.Query("from"), c.Query("to"))
	if err != nil {
		adminError(c, err)
		return
	}
	success(c, 200, gin.H{"items": value, "max_days": admin.MaxExportDays})
}

// export streams a dataset as CSV. Validation happens before any bytes are
// written, so errors are still reported as JSON.
func (h *adminHandler) export(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	key, from, to := c.Param("dataset"), c.Query("from"), c.Query("to")
	list, err := h.svc.Exporter.Datasets(c.Request.Context(), "", "")
	if err != nil {
		adminError(c, err)
		return
	}
	known := false
	for _, d := range list {
		if d.Key == key {
			known = true
			if d.Ranged {
				if _, _, ok := admin.ExportRange(from, to); !ok {
					adminError(c, admin.ErrInvalid)
					return
				}
			}
		}
	}
	if !known {
		adminError(c, admin.ErrNotFound)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": admin.FileName(key, from, to, time.Now())}))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	if _, err := h.svc.Exporter.Export(c.Request.Context(), c.Writer, adminUser(c).ID, key, from, to); err != nil {
		_ = c.Error(err) // headers are sent; the truncated download is the only signal
	}
}

func (h *adminHandler) backups(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	page, size, err := parsePage(c)
	if c.Query("page_size") == "" {
		size = 20
	}
	if err != nil || page < 1 || page > 10000 || size < 1 || size > 100 {
		adminError(c, admin.ErrInvalid)
		return
	}
	value, err := h.svc.Backups.List(c.Request.Context(), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *adminHandler) createBackup(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	actor := adminUser(c).ID
	value, err := h.svc.Backups.Create(c.Request.Context(), &actor, "manual")
	if err != nil {
		adminError(c, err)
		return
	}
	success(c, http.StatusCreated, value)
}

func (h *adminHandler) verifyBackup(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	value, err := h.svc.Backups.Verify(c.Request.Context(), adminUser(c).ID, c.Param("id"))
	if err != nil {
		adminError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *adminHandler) downloadBackup(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	f, run, err := h.svc.Backups.Open(c.Request.Context(), adminUser(c).ID, c.Param("id"))
	if err != nil {
		adminError(c, err)
		return
	}
	defer f.Close()
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Length", strconv.FormatInt(*run.SizeBytes, 10))
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": *run.FileName}))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, f)
}

func (h *adminHandler) deleteBackup(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if err := h.svc.Backups.Delete(c.Request.Context(), adminUser(c).ID, c.Param("id")); err != nil {
		adminError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}

func registerAdmin(g *gin.RouterGroup, service *auth.Service, svc *AdminServices, guard gin.HandlerFunc) {
	h := &adminHandler{svc: svc}
	perm := func(p string) gin.HandlerFunc { return RequirePermission(service, p) }
	g.GET("/admin/overview", perm("system:read"), guard, RequireAdministrator(), h.overview)
	g.GET("/admin/exports", perm("data:export"), guard, RequireAdministrator(), h.datasets)
	g.GET("/admin/exports/:dataset", perm("data:export"), guard, RequireAdministrator(), h.export)
	g.GET("/admin/backups", perm("backups:manage"), guard, RequireAdministrator(), h.backups)
	g.POST("/admin/backups", perm("backups:manage"), guard, RequireAdministrator(), h.createBackup)
	g.POST("/admin/backups/:id/verify", perm("backups:manage"), guard, RequireAdministrator(), h.verifyBackup)
	g.GET("/admin/backups/:id/download", perm("backups:manage"), guard, RequireAdministrator(), h.downloadBackup)
	g.DELETE("/admin/backups/:id", perm("backups:manage"), guard, RequireAdministrator(), h.deleteBackup)
}
