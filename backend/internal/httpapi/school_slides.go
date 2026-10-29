package httpapi

import (
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

type slideOrderRequest struct {
	IDs []string `json:"ids"`
}

// slides lists the public landing-page slideshow (no image bytes).
func (h *schoolHandler) slides(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.Slides(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	success(c, 200, value)
}

// slideImage serves one slideshow photo. A URL that carries the current
// version (?v=) never changes, so browsers and proxies may keep it for a year.
func (h *schoolHandler) slideImage(c *gin.Context) {
	if !h.available(c) {
		return
	}
	img, err := h.service.SlideImage(c.Request.Context(), c.Param("id"))
	if err != nil {
		schoolError(c, err)
		return
	}
	etag := `"` + img.Version + `"`
	if c.Query("v") == img.Version {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "public, max-age=300")
	}
	c.Header("ETag", etag)
	c.Header("X-Content-Type-Options", "nosniff")
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(200, img.ContentType, img.Content)
}

func (h *schoolHandler) addSlide(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (kind != "image/jpeg" && kind != "image/png") {
		failure(c, 415, "unsupported_media_type", "a JPEG or PNG image is required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, school.MaxSlideUpload+1))
	if err != nil || len(body) > school.MaxSlideUpload {
		failure(c, 413, "too_large", "the photo is too large")
		return
	}
	value, err := h.service.AddSlide(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, kind, body)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 201, value)
}

func (h *schoolHandler) updateSlide(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input school.SlideCaption
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	value, err := h.service.UpdateSlideCaption(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, c.Param("id"), input)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *schoolHandler) reorderSlides(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input slideOrderRequest
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	value, err := h.service.ReorderSlides(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, input.IDs)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, value)
}

func (h *schoolHandler) deleteSlide(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteSlide(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, c.Param("id")); err != nil {
		schoolError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}
