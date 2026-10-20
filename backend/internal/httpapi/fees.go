package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/fees"
)

type feeHandler struct{ service *fees.Service }

// RequireAnyRole complements RequirePermission for financial writes, so a
// mistaken grant to an unrelated role still cannot move money.
func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("auth_user")
		current, valid := user.(auth.User)
		if !ok || !valid {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		for _, role := range roles {
			if current.Role == role {
				c.Next()
				return
			}
		}
		failure(c, http.StatusForbidden, "forbidden", "permission denied")
		c.Abort()
	}
}

func (h *feeHandler) ready(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "fees_unavailable", "fees require a database")
	return false
}

func feeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, fees.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid fee input")
	case errors.Is(err, fees.ErrNotFound):
		failure(c, 404, "not_found", "fee record not found")
	case errors.Is(err, fees.ErrOverpayment):
		failure(c, 409, "overpayment", "amount exceeds the outstanding balance")
	case errors.Is(err, fees.ErrDuplicate):
		failure(c, 409, "duplicate_payment", "payment reference or request was already used")
	case errors.Is(err, fees.ErrConflict):
		failure(c, 409, "fee_conflict", "fee record is already settled, used or duplicated")
	case errors.Is(err, fees.ErrExpired):
		failure(c, 409, "payment_expired", "mobile payment session expired")
	case errors.Is(err, fees.ErrDeclined):
		failure(c, 422, "payment_declined", "mobile payment was declined")
	case errors.Is(err, fees.ErrForbidden):
		failure(c, 403, "forbidden", "fee access denied")
	case errors.Is(err, fees.ErrOnlineDisabled):
		failure(c, 403, "online_payments_disabled", "online payment is not enabled in this school")
	case errors.Is(err, fees.ErrSimulationNotAllowed):
		failure(c, 409, "simulation_not_allowed", "this installation does not allow payment simulation")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func feeUser(c *gin.Context) auth.User { return c.MustGet("auth_user").(auth.User) }

func feeQuery(c *gin.Context) (fees.Query, bool) {
	page, size, err := parsePage(c)
	overdue := c.Query("overdue_only")
	simulated := c.Query("simulated")
	if err != nil || (overdue != "" && overdue != "true" && overdue != "false") || (simulated != "" && simulated != "true" && simulated != "false" && simulated != "unreviewed") {
		feeError(c, fees.ErrInvalid)
		return fees.Query{}, false
	}
	return fees.Query{SessionID: c.Query("session_id"), ClassID: c.Query("class_id"), SectionID: c.Query("section_id"), StudentID: c.Query("student_id"), CategoryID: c.Query("category_id"),
		Status: c.Query("status"), Period: c.Query("period"), Method: c.Query("method"), EntryType: c.Query("type"), Category: c.Query("category"),
		From: c.Query("from"), To: c.Query("to"), Search: c.Query("search"), OverdueOnly: overdue == "true", Simulated: simulated, Page: page, PageSize: size}, true
}

// respond writes a read result or maps its error.
func respond[T any](c *gin.Context, status int, value T, err error) {
	if err != nil {
		feeError(c, err)
		return
	}
	success(c, status, value)
}

func created(c *gin.Context) int {
	if c.Param("id") == "" {
		return http.StatusCreated
	}
	return http.StatusOK
}

func (h *feeHandler) options(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Options(c.Request.Context())
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) students(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.SearchStudents(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) categories(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.ListCategories(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) saveCategory(c *gin.Context) {
	var in fees.CategoryInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.SaveCategory(c.Request.Context(), feeUser(c).ID, c.Param("id"), in)
		respond(c, created(c), v, err)
	}
}
func (h *feeHandler) structures(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.ListStructures(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) saveStructure(c *gin.Context) {
	var in fees.StructureInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.SaveStructure(c.Request.Context(), feeUser(c).ID, c.Param("id"), in)
		respond(c, created(c), v, err)
	}
}
func (h *feeHandler) concessions(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.ListConcessions(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) saveConcession(c *gin.Context) {
	var in fees.ConcessionInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.SaveConcession(c.Request.Context(), feeUser(c).ID, in)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) deleteConcession(c *gin.Context) {
	if h.ready(c) {
		respond(c, 200, gin.H{"deleted": true}, h.service.DeleteConcession(c.Request.Context(), feeUser(c).ID, c.Param("id")))
	}
}
func (h *feeHandler) dues(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.ListDues(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) assignDue(c *gin.Context) {
	var in fees.DueInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.AssignDue(c.Request.Context(), feeUser(c).ID, in)
		respond(c, http.StatusCreated, v, err)
	}
}
func (h *feeHandler) generateDues(c *gin.Context) {
	var in fees.GenerateInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.GenerateDues(c.Request.Context(), feeUser(c).ID, in)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) deleteDue(c *gin.Context) {
	if h.ready(c) {
		respond(c, 200, gin.H{"deleted": true}, h.service.DeleteDue(c.Request.Context(), feeUser(c).ID, c.Param("id")))
	}
}
func (h *feeHandler) adjustments(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Adjustments(c.Request.Context(), c.Param("id"))
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) adjust(c *gin.Context) {
	var in fees.AdjustmentInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.Adjust(c.Request.Context(), feeUser(c).ID, feeUser(c).Role, c.Param("id"), in)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) statement(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Statement(c.Request.Context(), "staff", feeUser(c).ID, c.Param("id"))
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) payments(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.ListPayments(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}

// recordPayment returns 201 for a new receipt and 200 when an idempotent
// retry replays the original receipt.
func (h *feeHandler) recordPayment(c *gin.Context) {
	var in fees.PaymentInput
	if h.ready(c) && decodeSchoolJSON(c, 16*1024, &in) {
		v, err := h.service.RecordPayment(c.Request.Context(), feeUser(c).ID, in)
		status := http.StatusCreated
		if v.Replayed {
			status = http.StatusOK
		}
		respond(c, status, v, err)
	}
}
func (h *feeHandler) receipt(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Receipt(c.Request.Context(), "staff", feeUser(c).ID, "", c.Param("id"))
		respond(c, 200, v, err)
	}
}

// scoped serves the student's own and guardian's linked-child views; the
// account role must match the scope as well as holding the permission.
func (h *feeHandler) scoped(c *gin.Context, scope string) bool {
	if !h.ready(c) {
		return false
	}
	if feeUser(c).Role != scope {
		feeError(c, fees.ErrForbidden)
		return false
	}
	return true
}
func (h *feeHandler) ownStatement(c *gin.Context) {
	if h.scoped(c, "student") {
		v, err := h.service.Statement(c.Request.Context(), "student", feeUser(c).ID, "")
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) ownReceipt(c *gin.Context) {
	if h.scoped(c, "student") {
		v, err := h.service.Receipt(c.Request.Context(), "student", feeUser(c).ID, "", c.Param("id"))
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) childStatement(c *gin.Context) {
	if h.scoped(c, "guardian") {
		v, err := h.service.Statement(c.Request.Context(), "guardian", feeUser(c).ID, c.Param("studentId"))
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) childReceipt(c *gin.Context) {
	if h.scoped(c, "guardian") {
		v, err := h.service.Receipt(c.Request.Context(), "guardian", feeUser(c).ID, c.Param("studentId"), c.Param("id"))
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) startOnline(c *gin.Context) {
	var in fees.IntentInput
	role := feeUser(c).Role
	if role != "student" && role != "guardian" {
		feeError(c, fees.ErrForbidden)
		return
	}
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.StartOnlinePayment(c.Request.Context(), role, feeUser(c).ID, in)
		respond(c, http.StatusCreated, v, err)
	}
}
func (h *feeHandler) confirmOnline(c *gin.Context) {
	var in fees.ConfirmInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.ConfirmOnlinePayment(c.Request.Context(), feeUser(c).ID, c.Param("id"), in)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) collections(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.Collections(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) dueReport(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.DueReport(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) entries(c *gin.Context) {
	if q, ok := feeQuery(c); ok && h.ready(c) {
		v, err := h.service.ListEntries(c.Request.Context(), q)
		respond(c, 200, v, err)
	}
}
func (h *feeHandler) saveEntry(c *gin.Context) {
	var in fees.EntryInput
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.service.SaveEntry(c.Request.Context(), feeUser(c).ID, in)
		respond(c, http.StatusCreated, v, err)
	}
}
func (h *feeHandler) deleteEntry(c *gin.Context) {
	if h.ready(c) {
		respond(c, 200, gin.H{"deleted": true}, h.service.DeleteEntry(c.Request.Context(), feeUser(c).ID, c.Param("id")))
	}
}
func (h *feeHandler) summary(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.Summary(c.Request.Context(), c.Query("from"), c.Query("to"))
		respond(c, 200, v, err)
	}
}

// onlineStatus tells any signed-in user whether online payment is available.
func (h *feeHandler) onlineStatus(c *gin.Context) {
	if h.ready(c) {
		v, err := h.service.OnlineStatus(c.Request.Context(), feeUser(c).Role == "admin")
		respond(c, 200, v, err)
	}
}

// saveOnlineSettings is the administrator's simulation switch.
func (h *feeHandler) saveOnlineSettings(c *gin.Context) {
	var in fees.OnlineSettingsInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.SaveOnlineSettings(c.Request.Context(), feeUser(c).ID, in)
		respond(c, 200, v, err)
	}
}

// voidSimulatedPayment restores the dues a simulated payment reduced.
func (h *feeHandler) voidSimulatedPayment(c *gin.Context) {
	var in fees.VoidInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.service.VoidSimulatedPayment(c.Request.Context(), feeUser(c).ID, c.Param("id"), in)
		respond(c, 200, v, err)
	}
}
