package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// The public school page (checkpoint 5). Reading it needs no sign-in and
// returns only what the administrator wrote for the public, plus optional
// totals; editing is for administrators.

func (h *schoolHandler) publicPage(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.PublicView(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	success(c, http.StatusOK, value)
}

// servePublicImage sends a stored JPEG; a URL with the current version never
// changes, so it may be cached for a year.
func servePublicImage(c *gin.Context, img school.SlideImage) {
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
	c.Data(http.StatusOK, img.ContentType, img.Content)
}

func (h *schoolHandler) headPhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	img, err := h.service.HeadPhoto(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	servePublicImage(c, img)
}

func (h *schoolHandler) galleryImage(c *gin.Context) {
	if !h.available(c) {
		return
	}
	img, err := h.service.GalleryImage(c.Request.Context(), c.Param("id"), c.Query("size") == "thumb")
	if err != nil {
		schoolError(c, err)
		return
	}
	servePublicImage(c, img)
}

func (h *schoolHandler) publicPageForEdit(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, gallery, err := h.service.PublicPageForEdit(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"page": page, "gallery": gallery})
}

func (h *schoolHandler) savePublicPage(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input school.PublicPageInput
	if !decodeSchoolJSON(c, 32*1024, &input) {
		return
	}
	value, err := h.service.SavePublicPage(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, input)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, value)
}

// publicUpload reads a JPEG or PNG upload for the public page.
func publicUpload(c *gin.Context) (string, []byte, bool) {
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (kind != "image/jpeg" && kind != "image/png") {
		failure(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "a JPEG or PNG image is required")
		return "", nil, false
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, school.MaxGalleryUpload+1))
	if err != nil || len(body) > school.MaxGalleryUpload {
		failure(c, http.StatusRequestEntityTooLarge, "too_large", "the photo is too large")
		return "", nil, false
	}
	return kind, body, true
}

func (h *schoolHandler) saveHeadPhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, body, ok := publicUpload(c)
	if !ok {
		return
	}
	value, err := h.service.SetHeadPhoto(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, kind, body)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, value)
}

func (h *schoolHandler) deleteHeadPhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	value, err := h.service.DeleteHeadPhoto(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, value)
}

func (h *schoolHandler) addGalleryPhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, body, ok := publicUpload(c)
	if !ok {
		return
	}
	value, err := h.service.AddGalleryPhoto(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, kind, body)
	if errors.Is(err, school.ErrGalleryLimit) {
		failure(c, http.StatusConflict, "gallery_limit", "the gallery already has the maximum number of photos")
		return
	}
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusCreated, value)
}

func (h *schoolHandler) updateGalleryPhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input school.SlideCaption
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	value, err := h.service.UpdateGalleryCaption(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, c.Param("id"), input)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, value)
}

func (h *schoolHandler) deleteGalleryPhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteGalleryPhoto(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, c.Param("id")); err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"deleted": true})
}
