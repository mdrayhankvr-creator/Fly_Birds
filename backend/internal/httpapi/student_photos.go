package httpapi

import (
	"io"
	"mime"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

// Students' own photo uploads and their review by the class teacher or the
// administrator. Registered before the /students/:id routes so "me" and
// "photo-reviews" are not read as student IDs.
func registerStudentPhotos(r *gin.RouterGroup, service *auth.Service, h *peopleHandler) {
	submit := RequirePermission(service, "students:photo_submit")
	review := RequirePermission(service, "students:photo_review")
	r.GET("/me/photo-submission", submit, h.myPhotoSubmission)
	r.GET("/me/photo-submission/image", submit, h.myPhotoSubmissionImage)
	r.PUT("/me/photo-submission", submit, h.submitPhoto)
	r.GET("/photo-reviews", review, h.photoReviews)
	r.GET("/:id/photo-submission/image", review, h.photoReviewImage)
	r.POST("/:id/photo-submission/approve", review, h.approvePhoto)
	r.POST("/:id/photo-submission/reject", review, h.rejectPhoto)
}

// photos answers 503 when the service runs without a database.
func (h *peopleHandler) photos(c *gin.Context) *people.PhotoSubmissions {
	if !h.available(c) {
		return nil
	}
	if h.service.Photos == nil {
		failure(c, 503, "service_unavailable", "student photos are unavailable")
		return nil
	}
	return h.service.Photos
}

func (h *peopleHandler) myPhotoSubmission(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	v, err := p.Mine(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}

func sendPhoto(c *gin.Context, data []byte) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, "image/jpeg", data)
}

func (h *peopleHandler) myPhotoSubmissionImage(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	data, err := p.MyImage(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	sendPhoto(c, data)
}

func (h *peopleHandler) submitPhoto(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (kind != "image/jpeg" && kind != "image/png") {
		failure(c, 415, "unsupported_media_type", "a JPEG or PNG image is required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, staff.MaxPhotoUpload+1))
	if err != nil || len(body) > staff.MaxPhotoUpload {
		failure(c, 413, "too_large", "the photo is too large")
		return
	}
	v, err := p.Submit(c.Request.Context(), actorFrom(c), kind, body)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *peopleHandler) photoReviews(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	items, err := p.Pending(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"items": items, "total": len(items)})
}

func (h *peopleHandler) photoReviewImage(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	data, err := p.ReviewImage(c.Request.Context(), actorFrom(c), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	sendPhoto(c, data)
}

func (h *peopleHandler) approvePhoto(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	if err := p.Approve(c.Request.Context(), actorFrom(c), c.Param("id")); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"status": "approved"})
}

func (h *peopleHandler) rejectPhoto(c *gin.Context) {
	p := h.photos(c)
	if p == nil {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeSchoolJSON(c, 4*1024, &input) {
		return
	}
	if err := p.Reject(c.Request.Context(), actorFrom(c), c.Param("id"), input.Reason); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"status": "rejected"})
}
