package httpapi

import (
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	qrcode "github.com/skip2/go-qrcode"
)

var cardOrigin = regexp.MustCompile(`^https?://[A-Za-z0-9.-]+(:[0-9]{1,5})?$`)

// verifyOrigin is the web address printed in card QR codes. The browser sends
// its own origin because a development proxy hides it; anything that is not
// a bare http(s) origin falls back to the request's host.
func verifyOrigin(c *gin.Context) string {
	if origin := c.Query("origin"); cardOrigin.MatchString(origin) {
		return origin
	}
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request.Host
	if !cardOrigin.MatchString(scheme + "://" + host) {
		host = "localhost"
	}
	return scheme + "://" + host
}

type cardView struct {
	Student    people.Student `json:"student"`
	ValidUntil string         `json:"valid_until"`
	QRPayload  string         `json:"qr_payload"`
	QRDataURL  string         `json:"qr_data_url"`
}

// cardQR is the verification link; QR attendance scanning accepts it too.
func cardQR(origin string, card people.Card) (cardView, error) {
	payload := origin + "/verify/" + card.Token
	png, err := qrcode.Encode(payload, qrcode.Medium, 256)
	if err != nil {
		return cardView{}, err
	}
	return cardView{Student: card.Student, ValidUntil: card.ValidUntil, QRPayload: payload, QRDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, nil
}

func (h *peopleHandler) cardContext(c *gin.Context) (people.CardSchool, people.CardSettings, bool) {
	schoolInfo, err := h.service.CardSchool(c.Request.Context())
	if err != nil {
		peopleError(c, err)
		return schoolInfo, people.CardSettings{}, false
	}
	settings, err := h.service.CardSettings(c.Request.Context())
	if err != nil {
		peopleError(c, err)
		return schoolInfo, settings, false
	}
	return schoolInfo, settings, true
}

// card godoc
// @Summary Read one student's ID card data
// @Tags students
// @Router /students/{id}/card [get]
func (h *peopleHandler) card(c *gin.Context) {
	if !h.available(c) {
		return
	}
	card, err := h.service.StudentCard(c.Request.Context(), actorFrom(c), c.Param("id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	schoolInfo, settings, ok := h.cardContext(c)
	if !ok {
		return
	}
	view, err := cardQR(verifyOrigin(c), card)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"student": view.Student, "valid_until": view.ValidUntil, "qr_payload": view.QRPayload, "qr_data_url": view.QRDataURL, "school": schoolInfo, "settings": settings})
}

// sectionCards godoc
// @Summary Read ID card data for every active student in a section
// @Tags students
// @Router /students/cards [get]
func (h *peopleHandler) sectionCards(c *gin.Context) {
	if !h.available(c) {
		return
	}
	cards, err := h.service.SectionCards(c.Request.Context(), actorFrom(c), c.Query("section_id"))
	if err != nil {
		peopleError(c, err)
		return
	}
	schoolInfo, settings, ok := h.cardContext(c)
	if !ok {
		return
	}
	origin := verifyOrigin(c)
	views := make([]cardView, 0, len(cards))
	for _, card := range cards {
		view, err := cardQR(origin, card)
		if err != nil {
			peopleError(c, err)
			return
		}
		views = append(views, view)
	}
	success(c, 200, gin.H{"cards": views, "school": schoolInfo, "settings": settings})
}

// reissueCard godoc
// @Summary Replace a student's card verification token
// @Tags students
// @Router /students/{id}/card/reissue [post]
func (h *peopleHandler) reissueCard(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.ReissueCard(c.Request.Context(), actorFrom(c), c.Param("id")); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"reissued": true})
}

// cardSettings godoc
// @Summary Read ID card settings
// @Tags students
// @Router /students/card-settings [get]
func (h *peopleHandler) cardSettings(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.CardSettings(c.Request.Context())
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}

// saveCardSettings godoc
// @Summary Update ID card settings
// @Tags students
// @Router /students/card-settings [put]
func (h *peopleHandler) saveCardSettings(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var in people.CardSettingsInput
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.SaveCardSettings(c.Request.Context(), actorFrom(c), in)
	if err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, v)
}

// signature godoc
// @Summary Read the signatory's signature image
// @Tags students
// @Router /students/card-settings/signature [get]
func (h *peopleHandler) signature(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.Signature(c.Request.Context(), actorFrom(c))
	if err != nil {
		peopleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, v.ContentType, v.Content)
}

// saveSignature godoc
// @Summary Upload the signatory's signature image
// @Tags students
// @Router /students/card-settings/signature [put]
func (h *peopleHandler) saveSignature(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		peopleError(c, people.ErrInvalid)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 524289))
	if err != nil {
		peopleError(c, people.ErrInvalid)
		return
	}
	if err := h.service.SaveSignature(c.Request.Context(), actorFrom(c), people.Photo{ContentType: kind, Content: body}); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"saved": true})
}

// deleteSignature godoc
// @Summary Remove the signatory's signature image
// @Tags students
// @Router /students/card-settings/signature [delete]
func (h *peopleHandler) deleteSignature(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteSignature(c.Request.Context(), actorFrom(c)); err != nil {
		peopleError(c, err)
		return
	}
	success(c, 200, gin.H{"removed": true})
}

// verifyCard godoc
// @Summary Publicly check a student ID card; reveals no personal data
// @Tags students
// @Router /public/id-cards/{token} [get]
func verifyCard(h *peopleHandler, schoolService *school.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Robots-Tag", "noindex")
		if h.service == nil || schoolService == nil {
			failure(c, 503, "people_unavailable", "student records require a database")
			return
		}
		// A disabled Students module looks the same as an unknown card.
		if enabled, err := schoolService.Enabled(c.Request.Context(), "students"); err != nil || !enabled {
			failure(c, 404, "not_found", "card not found")
			return
		}
		v, err := h.service.VerifyCard(c.Request.Context(), c.Param("token"))
		if errors.Is(err, people.ErrNotFound) {
			failure(c, 404, "not_found", "card not found")
			return
		}
		if err != nil {
			failure(c, 500, "internal_error", "internal server error")
			return
		}
		success(c, 200, v)
	}
}
