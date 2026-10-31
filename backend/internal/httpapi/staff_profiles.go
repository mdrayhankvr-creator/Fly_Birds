package httpapi

import (
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

// Staff profiles (checkpoint 4). The service removes each field the viewer
// may not see; these handlers only pass the viewer through.

func staffViewer(c *gin.Context) staff.Viewer {
	u := c.MustGet("auth_user").(auth.User)
	return staff.Viewer{ID: u.ID, Role: u.Role}
}

func (h *staffHandler) profile(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Profile(c.Request.Context(), staffViewer(c), c.Param("id"))
	if err != nil {
		staffError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	success(c, http.StatusOK, v)
}

func (h *staffHandler) ownProfile(c *gin.Context) {
	if !h.available(c) {
		return
	}
	viewer := staffViewer(c)
	id, err := h.service.OwnProfileID(c.Request.Context(), viewer)
	if err != nil {
		staffError(c, err)
		return
	}
	v, err := h.service.Profile(c.Request.Context(), viewer, id)
	if err != nil {
		staffError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	success(c, http.StatusOK, v)
}

func (h *staffHandler) saveOwnProfile(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in staff.OwnProfileInput
	if !decodeSchoolJSON(c, 16*1024, &in) {
		return
	}
	v, err := h.service.SaveOwnProfile(c.Request.Context(), staffViewer(c), in)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, v)
}

func (h *staffHandler) saveOfficial(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in staff.OfficialInput
	if !decodeSchoolJSON(c, 4*1024, &in) {
		return
	}
	v, err := h.service.SaveOfficial(c.Request.Context(), staffViewer(c), c.Param("id"), in)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, v)
}

func (h *staffHandler) revealNID(c *gin.Context) {
	if !h.available(c) {
		return
	}
	nid, err := h.service.RevealNID(c.Request.Context(), staffViewer(c), c.Param("id"))
	if err != nil {
		staffError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	success(c, http.StatusOK, gin.H{"nid": nid})
}

func (h *staffHandler) removeNID(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.RemoveNID(c.Request.Context(), staffViewer(c), c.Param("id"))
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, v)
}

// photoTarget is the employee whose photo a request changes: the caller's
// own record on /staff/me/photo, or the one in the path for administrators.
func (h *staffHandler) photoTarget(c *gin.Context) (string, bool) {
	if id := c.Param("id"); id != "" {
		return id, true
	}
	id, err := h.service.OwnProfileID(c.Request.Context(), staffViewer(c))
	if err != nil {
		staffError(c, err)
		return "", false
	}
	return id, true
}

func (h *staffHandler) savePhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	id, ok := h.photoTarget(c)
	if !ok {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, staff.MaxPhotoUpload+1))
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	v, err := h.service.SavePhoto(c.Request.Context(), staffViewer(c), id, kind, body)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, v)
}

func (h *staffHandler) deletePhoto(c *gin.Context) {
	if !h.available(c) {
		return
	}
	id, ok := h.photoTarget(c)
	if !ok {
		return
	}
	if err := h.service.DeletePhoto(c.Request.Context(), staffViewer(c), id); err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"deleted": true})
}

// photo serves a staff photo to a signed-in viewer who may open the profile.
// Cached privately: the version in the URL changes when the photo does.
func (h *staffHandler) photo(c *gin.Context) {
	if !h.available(c) {
		return
	}
	content, version, err := h.service.Photo(c.Request.Context(), staffViewer(c), c.Param("id"))
	if err != nil {
		staffError(c, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("ETag", `"`+version+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "image/jpeg", content)
}
