package httpapi

import (
	"errors"
	"io"
	"mime"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// Result documents (E7): PDF marksheets and class documents.
type documentHandler struct{ service *documents.Service }

func (h *documentHandler) ready(c *gin.Context) bool {
	if h.service.Available() {
		return true
	}
	failure(c, 503, "documents_unavailable", "the document renderer is not available")
	return false
}

func documentActor(c *gin.Context) documents.Actor {
	u := c.MustGet("auth_user").(auth.User)
	return documents.Actor{ID: u.ID, Role: u.Role}
}

func documentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, documents.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid document request")
	case errors.Is(err, documents.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	case errors.Is(err, documents.ErrNotFound):
		failure(c, 404, "not_found", "document not found")
	case errors.Is(err, documents.ErrNotPublished):
		failure(c, 409, "results_not_published", "these results are not published")
	case errors.Is(err, documents.ErrNotReady):
		failure(c, 409, "document_not_ready", "the document is not ready")
	case errors.Is(err, documents.ErrUnavailable):
		failure(c, 503, "documents_unavailable", "the document renderer is not available")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func (h *documentHandler) answer(c *gin.Context, v any, err error) {
	if err != nil {
		documentError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *documentHandler) classes(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Classes(c.Request.Context(), documentActor(c))
		h.answer(c, v, err)
	}
}

func (h *documentHandler) create(c *gin.Context) {
	var in documents.Request
	if h.ready(c) && decodeSchoolJSON(c, 2*1024, &in) {
		v, err := h.service.Request(c.Request.Context(), documentActor(c), in)
		h.answer(c, v, err)
	}
}

func (h *documentHandler) get(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Get(c.Request.Context(), documentActor(c), c.Param("id"))
		h.answer(c, v, err)
	}
}

func (h *documentHandler) file(c *gin.Context) {
	if h.ready(c) {
		doc, rc, err := h.service.Open(c.Request.Context(), documentActor(c), c.Param("id"))
		sendDocument(c, doc, rc, err)
	}
}

// mine is a student's own marksheet of a current published result: it is
// prepared (or taken from the cache) and sent at once.
func (h *documentHandler) mine(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	a := documentActor(c)
	doc, err := h.service.StudentMarksheet(c.Request.Context(), a, c.Param("versionId"), c.DefaultQuery("lang", "en"))
	if err != nil {
		documentError(c, err)
		return
	}
	if doc.Status != "done" {
		documentError(c, documents.ErrNotReady)
		return
	}
	doc, rc, err := h.service.Open(c.Request.Context(), a, doc.ID)
	sendDocument(c, doc, rc, err)
}

func sendDocument(c *gin.Context, doc documents.Document, rc io.ReadCloser, err error) {
	if err != nil {
		documentError(c, err)
		return
	}
	defer rc.Close()
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": doc.FileName}))
	if doc.Bytes != nil {
		c.Header("Content-Length", strconv.FormatInt(*doc.Bytes, 10))
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(200)
	_, _ = io.Copy(c.Writer, rc)
}

// verifyResult is the public check behind a marksheet's QR code.
func verifyResult(h *documentHandler, schoolService *school.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Robots-Tag", "noindex")
		if h.service == nil || schoolService == nil {
			failure(c, 404, "not_found", "result not found")
			return
		}
		// A disabled Examinations module looks the same as an unknown code.
		if enabled, err := schoolService.Enabled(c.Request.Context(), "examinations"); err != nil || !enabled {
			failure(c, 404, "not_found", "result not found")
			return
		}
		v, err := h.service.Verify(c.Request.Context(), c.Param("token"))
		if errors.Is(err, documents.ErrNotFound) {
			failure(c, 404, "not_found", "result not found")
			return
		}
		if err != nil {
			failure(c, 500, "internal_error", "internal server error")
			return
		}
		success(c, 200, v)
	}
}

func registerDocuments(router *gin.Engine, service *auth.Service, schoolService *school.Service, h *documentHandler, examsModule gin.HandlerFunc) {
	g := router.Group("/exams")
	g.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service), examsModule)
	staff := RequirePermission(service, "exams:documents")
	g.GET("/documents/classes", staff, h.classes)
	g.POST("/documents", staff, h.create)
	g.GET("/documents/:id", staff, h.get)
	g.GET("/documents/:id/file", staff, h.file)
	g.GET("/me/documents/:versionId", RequirePermission(service, "exams:read_self"), h.mine)
	router.GET("/public/results/:token", verifyResult(h, schoolService))
}
