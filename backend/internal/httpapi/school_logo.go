package httpapi

import (
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// The uploaded school logo: public to read (it is on the sign-in page and
// every document), administrators upload and remove it.

func (h *schoolHandler) logos(c *gin.Context) *school.LogoStore {
	if !h.available(c) {
		return nil
	}
	if h.service.Logos == nil {
		failure(c, http.StatusServiceUnavailable, "service_unavailable", "the logo is unavailable")
		return nil
	}
	return h.service.Logos
}

func (h *schoolHandler) logo(c *gin.Context) {
	l := h.logos(c)
	if l == nil {
		return
	}
	logo, err := l.Get(c.Request.Context())
	if err != nil {
		schoolError(c, err)
		return
	}
	servePublicImage(c, school.SlideImage{ContentType: logo.ContentType, Content: logo.Content, Version: logo.Version})
}

func (h *schoolHandler) saveLogo(c *gin.Context) {
	l := h.logos(c)
	if l == nil {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (kind != "image/jpeg" && kind != "image/png") {
		failure(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "a JPEG or PNG image is required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, school.MaxLogoUpload+1))
	if err != nil || len(body) > school.MaxLogoUpload {
		failure(c, http.StatusRequestEntityTooLarge, "too_large", "the logo is too large")
		return
	}
	logoURL, err := l.Save(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID, kind, body)
	if err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"logo_url": logoURL})
}

func (h *schoolHandler) deleteLogo(c *gin.Context) {
	l := h.logos(c)
	if l == nil {
		return
	}
	if err := l.Delete(c.Request.Context(), c.MustGet("auth_user").(auth.User).ID); err != nil {
		schoolError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"logo_url": ""})
}
