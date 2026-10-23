package httpapi

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

// E6: result versions, review, approval, publication and corrections.

type resultHandler struct{ service *exams.Service }

func (h *resultHandler) results(c *gin.Context) *exams.Results {
	if h.service != nil {
		if r := h.service.Results(); r != nil {
			return r
		}
	}
	failure(c, 503, "exams_unavailable", "exams require a database")
	return nil
}

// resultError maps the workflow's errors to specific codes, then falls back
// to the examination errors.
func resultError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, exams.ErrResultsNotReady):
		failure(c, 409, "results_not_ready", "some results are incomplete or sheets await approval")
	case errors.Is(err, exams.ErrReviewStale):
		failure(c, 409, "review_stale", "the results changed since you reviewed them; review them again")
	case errors.Is(err, exams.ErrApprovalStale):
		failure(c, 409, "approval_stale", "the results changed since they were approved; they must be approved again")
	case errors.Is(err, exams.ErrNotApproved):
		failure(c, 409, "results_not_approved", "the results have not been approved")
	case errors.Is(err, exams.ErrNothingToApprove):
		failure(c, 409, "nothing_to_approve", "nothing changed since the results were published")
	case errors.Is(err, exams.ErrAlreadyPublished):
		failure(c, 409, "already_published", "these results are already published")
	case errors.Is(err, exams.ErrNotPublished):
		failure(c, 409, "results_not_published", "the results are not published yet")
	case errors.Is(err, exams.ErrReasonRequired):
		failure(c, 400, "reason_required", "give a reason of 3 to 500 characters")
	default:
		examError(c, err)
	}
}

func scopeRef(c *gin.Context) exams.ScopeRef {
	return exams.ScopeRef{Scope: c.Query("scope"), ScopeID: c.Query("scope_id"), ClassID: c.Query("class_id")}
}

func (h *resultHandler) overview(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	v, err := r.Overview(c.Request.Context(), c.Query("session_id"))
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) review(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	v, err := r.Review(c.Request.Context(), scopeRef(c))
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) approve(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	var in exams.ApproveInput
	if !decodeSchoolJSON(c, 4096, &in) {
		return
	}
	v, err := r.Approve(c.Request.Context(), examUser(c).ID, in)
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) publish(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	var in exams.PublishInput
	if !decodeSchoolJSON(c, 4096, &in) {
		return
	}
	v, err := r.Publish(c.Request.Context(), examUser(c).ID, in)
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) history(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	v, err := r.History(c.Request.Context(), c.Query("scope"), c.Query("scope_id"), c.Query("student_id"))
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) version(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	v, err := r.Version(c.Request.Context(), c.Param("versionId"))
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) corrections(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	v, err := r.Corrections(c.Request.Context(), c.Param("id"))
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *resultHandler) openCorrection(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	var in struct {
		StudentID string `json:"student_id"`
		Reason    string `json:"reason"`
	}
	if !decodeSchoolJSON(c, 4096, &in) {
		return
	}
	v, err := r.OpenCorrection(c.Request.Context(), examUser(c).ID, c.Param("id"), in.StudentID, in.Reason)
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 201, v)
}

func (h *resultHandler) cancelCorrection(c *gin.Context) {
	r := h.results(c)
	if r == nil {
		return
	}
	if err := r.CancelCorrection(c.Request.Context(), examUser(c).ID, c.Param("id"), c.Param("correctionId")); err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, gin.H{"cancelled": true})
}

func (h *resultHandler) mine(c *gin.Context)     { h.publishedFor(c, "student", "") }
func (h *resultHandler) children(c *gin.Context) { h.publishedFor(c, "guardian", c.Param("studentId")) }

// publishedFor lists a student's published term and annual results for the
// student or a linked guardian.
func (h *resultHandler) publishedFor(c *gin.Context, scope, studentID string) {
	r := h.results(c)
	if r == nil {
		return
	}
	if examUser(c).Role != scope {
		examError(c, exams.ErrForbidden)
		return
	}
	v, err := r.PublishedResults(c.Request.Context(), scope, examUser(c).ID, studentID)
	if err != nil {
		resultError(c, err)
		return
	}
	success(c, 200, v)
}
