package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/classteacher"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

// Marksheet v2 and admit cards: class teachers and their comments, staff
// signatures, and admit cards made from published term routines.

// ---------------------------------------------------------------------------
// Class teachers and comments

type classTeacherHandler struct{ service *classteacher.Service }

func (h *classTeacherHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "unavailable", "class teachers need a database")
	return false
}

func classTeacherActor(c *gin.Context) classteacher.Actor {
	u := c.MustGet("auth_user").(auth.User)
	return classteacher.Actor{ID: u.ID, Role: u.Role}
}

func classTeacherError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, classteacher.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid request")
	case errors.Is(err, classteacher.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	case errors.Is(err, classteacher.ErrNotFound):
		failure(c, 404, "not_found", "not found")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func (h *classTeacherHandler) answer(c *gin.Context, v any, err error) {
	if err != nil {
		classTeacherError(c, err)
		return
	}
	success(c, 200, v)
}

// @Summary List a session's sections with their class teachers
// @Tags academic
// @Router /academic/class-teachers [get]
func (h *classTeacherHandler) sections(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Sections(c.Request.Context(), c.Query("session_id"))
		h.answer(c, v, err)
	}
}

// @Summary Assign or clear a section's class teacher
// @Tags academic
// @Router /academic/sections/{id}/class-teacher [put]
func (h *classTeacherHandler) assign(c *gin.Context) {
	var in struct {
		EmployeeID string `json:"employee_id"`
	}
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.Assign(c.Request.Context(), classTeacherActor(c), c.Param("id"), strings.TrimSpace(in.EmployeeID))
		h.answer(c, v, err)
	}
}

// @Summary List preset comments
// @Tags exams
// @Router /exams/comments/presets [get]
func (h *classTeacherHandler) presets(c *gin.Context) {
	if h.ready(c) {
		a := classTeacherActor(c)
		v, err := h.service.Presets(c.Request.Context(), c.Query("all") == "true" && (a.Role == "admin" || a.Role == "head_teacher"))
		h.answer(c, v, err)
	}
}

// @Summary Create or change a preset comment
// @Tags exams
// @Router /exams/comments/presets [post]
func (h *classTeacherHandler) savePreset(c *gin.Context) {
	var in classteacher.PresetInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.SavePreset(c.Request.Context(), classTeacherActor(c), c.Param("id"), in)
		h.answer(c, v, err)
	}
}

// @Summary Sections the signed-in teacher may comment on or review
// @Tags exams
// @Router /exams/comments/sections [get]
func (h *classTeacherHandler) mySections(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.MySections(c.Request.Context(), classTeacherActor(c), c.Query("session_id"))
		h.answer(c, v, err)
	}
}

// @Summary A section's class-teacher comments on one result
// @Tags exams
// @Router /exams/comments [get]
func (h *classTeacherHandler) sheet(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Sheet(c.Request.Context(), classTeacherActor(c), c.Query("scope"), c.Query("scope_id"), c.Query("section_id"))
		h.answer(c, v, err)
	}
}

// @Summary Write or clear a student's class-teacher comment
// @Tags exams
// @Router /exams/comments [put]
func (h *classTeacherHandler) save(c *gin.Context) {
	var in classteacher.CommentInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.Save(c.Request.Context(), classTeacherActor(c), in)
		h.answer(c, v, err)
	}
}

// @Summary Mark a section's comments reviewed
// @Tags exams
// @Router /exams/comments/review [post]
func (h *classTeacherHandler) review(c *gin.Context) {
	var in classteacher.ReviewInput
	if h.ready(c) && decodeSchoolJSON(c, 32*1024, &in) {
		n, err := h.service.Review(c.Request.Context(), classTeacherActor(c), in)
		h.answer(c, gin.H{"reviewed": n}, err)
	}
}

// ---------------------------------------------------------------------------
// Signatures

// @Summary The signed-in member of staff's signature (PNG)
// @Tags staff
// @Router /staff/me/signature [get]
func (h *staffHandler) signature(c *gin.Context) {
	if !h.available(c) {
		return
	}
	content, version, err := h.service.OwnSignature(c.Request.Context(), staffViewer(c))
	if err != nil {
		staffError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("ETag", `"`+version+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "image/png", content)
}

// @Summary Upload the signed-in member of staff's signature
// @Tags staff
// @Router /staff/me/signature [put]
func (h *staffHandler) saveSignature(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, staff.MaxSignatureUpload+1))
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	version, err := h.service.SaveOwnSignature(c.Request.Context(), staffViewer(c), kind, body)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"version": version})
}

// @Summary Remove the signed-in member of staff's signature
// @Tags staff
// @Router /staff/me/signature [delete]
func (h *staffHandler) deleteSignature(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if err := h.service.DeleteOwnSignature(c.Request.Context(), staffViewer(c)); err != nil {
		staffError(c, err)
		return
	}
	success(c, http.StatusOK, gin.H{"deleted": true})
}

// ---------------------------------------------------------------------------
// Admit cards

type admitHandler struct{ service *routine.AdmitService }

func (h *admitHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "routine_unavailable", "admit cards need a database")
	return false
}

func admitActor(c *gin.Context) routine.AdmitActor {
	u := c.MustGet("auth_user").(auth.User)
	return routine.AdmitActor{ID: u.ID, Role: u.Role}
}

func admitError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, routine.ErrHeld):
		failure(c, 409, "admit_card_held", "the admit card is held until dues are paid; please contact the school office")
	case errors.Is(err, routine.ErrNoCards):
		failure(c, 409, "no_admit_cards", "no admit card can be printed: every student is held for dues, or there are none")
	case errors.Is(err, routine.ErrDocuments):
		failure(c, 503, "documents_unavailable", "admit cards need the document renderer")
	default:
		routineError(c, err)
	}
}

func (h *admitHandler) answer(c *gin.Context, v any, err error) {
	if err != nil {
		admitError(c, err)
		return
	}
	success(c, 200, v)
}

// @Summary Admit card rules and the fee hold
// @Tags routine
// @Router /routines/admit-cards/settings [get]
func (h *admitHandler) settings(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Settings(c.Request.Context())
		h.answer(c, v, err)
	}
}

// @Summary Change the admit card rules and the fee hold
// @Tags routine
// @Router /routines/admit-cards/settings [put]
func (h *admitHandler) saveSettings(c *gin.Context) {
	var in routine.AdmitSettings
	if h.ready(c) && decodeSchoolJSON(c, 16*1024, &in) {
		v, err := h.service.SaveSettings(c.Request.Context(), admitActor(c), in)
		h.answer(c, v, err)
	}
}

// @Summary Terms with a published routine and the sections the actor may print
// @Tags routine
// @Router /routines/admit-cards/terms [get]
func (h *admitHandler) terms(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Terms(c.Request.Context(), admitActor(c))
		h.answer(c, v, err)
	}
}

// @Summary A term's students with the fee hold
// @Tags routine
// @Router /routines/admit-cards/terms/{termId}/students [get]
func (h *admitHandler) students(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Students(c.Request.Context(), admitActor(c), c.Param("termId"), c.Query("class_id"), c.Query("section_id"), false)
		h.answer(c, v, err)
	}
}

// @Summary Print the admit cards of a section or class (PDF)
// @Tags routine
// @Router /routines/admit-cards/terms/{termId}/pdf [get]
func (h *admitHandler) pdf(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	lang := c.DefaultQuery("lang", "en")
	pdf, held, err := h.service.Print(c.Request.Context(), admitActor(c), c.Param("termId"), routine.AdmitPrint{ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), Language: lang})
	if err != nil {
		admitError(c, err)
		return
	}
	// How many students were left out for dues.
	c.Header("X-Admit-Cards-Held", fmt.Sprint(held))
	sendPDF(c, "admit-cards-"+lang+".pdf", pdf, nil)
}

// @Summary Students with dues by the cutoff (held or released)
// @Tags routine
// @Router /routines/admit-cards/withheld [get]
func (h *admitHandler) withheld(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Students(c.Request.Context(), admitActor(c), c.Query("term_id"), "", "", true)
		h.answer(c, v, err)
	}
}

// @Summary Release a held student's admit card for a term
// @Tags routine
// @Router /routines/admit-cards/terms/{termId}/releases/{studentId} [put]
func (h *admitHandler) release(c *gin.Context) {
	var in struct {
		Note string `json:"note"`
	}
	if h.ready(c) && decodeSchoolJSON(c, 2*1024, &in) {
		err := h.service.Release(c.Request.Context(), admitActor(c), c.Param("termId"), c.Param("studentId"), in.Note)
		h.answer(c, gin.H{"released": true}, err)
	}
}

// @Summary Hold a released student's admit card again
// @Tags routine
// @Router /routines/admit-cards/terms/{termId}/releases/{studentId} [delete]
func (h *admitHandler) unrelease(c *gin.Context) {
	if h.ready(c) {
		err := h.service.Unrelease(c.Request.Context(), admitActor(c), c.Param("termId"), c.Param("studentId"))
		h.answer(c, gin.H{"released": false}, err)
	}
}

// @Summary The signed-in student's admit cards
// @Tags routine
// @Router /routines/admit-cards/me [get]
func (h *admitHandler) mine(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if routineUser(c).Role != "student" {
		failure(c, 403, "forbidden", "own admit cards are for students")
		return
	}
	v, err := h.service.Mine(c.Request.Context(), actorID(c))
	h.answer(c, v, err)
}

// @Summary The signed-in student's admit card of a term (PDF)
// @Tags routine
// @Router /routines/admit-cards/me/{termId}/pdf [get]
func (h *admitHandler) minePDF(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if routineUser(c).Role != "student" {
		failure(c, 403, "forbidden", "own admit cards are for students")
		return
	}
	lang := c.DefaultQuery("lang", "en")
	pdf, err := h.service.MinePDF(c.Request.Context(), actorID(c), c.Param("termId"), lang)
	if err != nil {
		admitError(c, err)
		return
	}
	sendPDF(c, "admit-card-"+lang+".pdf", pdf, nil)
}

// verifyAdmit is the public check behind an admit card's QR code.
func verifyAdmit(h *admitHandler, schoolService *school.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Robots-Tag", "noindex")
		if h.service == nil || schoolService == nil {
			failure(c, 404, "not_found", "admit card not found")
			return
		}
		// A disabled Routine module looks the same as an unknown code.
		if enabled, err := schoolService.Enabled(c.Request.Context(), "routine"); err != nil || !enabled {
			failure(c, 404, "not_found", "admit card not found")
			return
		}
		v, err := h.service.Verify(c.Request.Context(), c.Param("token"))
		if errors.Is(err, routine.ErrNotFound) {
			failure(c, 404, "not_found", "admit card not found")
			return
		}
		if err != nil {
			failure(c, 500, "internal_error", "internal server error")
			return
		}
		success(c, 200, v)
	}
}

// registerMarksheetAdmit adds the routes of this file.
func registerMarksheetAdmit(router *gin.Engine, management *gin.RouterGroup, service *auth.Service, schoolService *school.Service, moduleGuard func(string) gin.HandlerFunc,
	ct *classteacher.Service, staffH *staffHandler, admit *routine.AdmitService) {
	ch := &classTeacherHandler{service: ct}
	management.GET("/academic/class-teachers", RequirePermission(service, "academics:read"), moduleGuard("academics"), ch.sections)
	management.PUT("/academic/sections/:id/class-teacher", RequirePermission(service, "academics:manage"), moduleGuard("academics"), RequireAdministrator(), ch.assign)
	comment := func(h gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{RequirePermission(service, "exams:comment"), moduleGuard("examinations"), h}
	}
	reviewer := func(h gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{RequirePermission(service, "exams:comment"), moduleGuard("examinations"), RequireAnyRole("admin", "head_teacher"), h}
	}
	management.GET("/exams/comments/presets", comment(ch.presets)...)
	management.POST("/exams/comments/presets", reviewer(ch.savePreset)...)
	management.PUT("/exams/comments/presets/:id", reviewer(ch.savePreset)...)
	management.GET("/exams/comments/sections", comment(ch.mySections)...)
	management.GET("/exams/comments", comment(ch.sheet)...)
	management.PUT("/exams/comments", comment(ch.save)...)
	management.POST("/exams/comments/review", reviewer(ch.review)...)

	management.GET("/staff/me/signature", RequirePermission(service, "staff:read_self"), moduleGuard("staff"), staffH.signature)
	management.PUT("/staff/me/signature", RequirePermission(service, "staff:read_self"), moduleGuard("staff"), staffH.saveSignature)
	management.DELETE("/staff/me/signature", RequirePermission(service, "staff:read_self"), moduleGuard("staff"), staffH.deleteSignature)

	ah := &admitHandler{service: admit}
	cards := func(h gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{RequirePermission(service, "routine:admit_cards"), moduleGuard("routine"), h}
	}
	office := func(h gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{RequirePermission(service, "routine:admit_release"), moduleGuard("routine"), h}
	}
	management.GET("/routines/admit-cards/settings", RequireAnyPermission(service, "routine:admit_cards", "routine:admit_release"), moduleGuard("routine"), ah.settings)
	management.PUT("/routines/admit-cards/settings", RequirePermission(service, "routine:admit_settings"), moduleGuard("routine"), RequireAdministrator(), ah.saveSettings)
	management.GET("/routines/admit-cards/terms", RequireAnyPermission(service, "routine:admit_cards", "routine:admit_release"), moduleGuard("routine"), ah.terms)
	management.GET("/routines/admit-cards/terms/:termId/students", cards(ah.students)...)
	management.GET("/routines/admit-cards/terms/:termId/pdf", cards(ah.pdf)...)
	management.GET("/routines/admit-cards/withheld", office(ah.withheld)...)
	management.PUT("/routines/admit-cards/terms/:termId/releases/:studentId", office(ah.release)...)
	management.DELETE("/routines/admit-cards/terms/:termId/releases/:studentId", office(ah.unrelease)...)
	management.GET("/routines/admit-cards/me", RequirePermission(service, "routine:read_self"), moduleGuard("routine"), ah.mine)
	management.GET("/routines/admit-cards/me/:termId/pdf", RequirePermission(service, "routine:read_self"), moduleGuard("routine"), ah.minePDF)
	router.GET("/public/admit-cards/:token", verifyAdmit(ah, schoolService))
}
