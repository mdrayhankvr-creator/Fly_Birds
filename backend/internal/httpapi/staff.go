package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

type staffHandler struct{ service *staff.Service }

func staffSubQuery(c *gin.Context) (staff.Query, error) {
	page, size, err := parsePage(c)
	return staff.Query{Search: c.Query("search"), Page: page, PageSize: size}, err
}

func (h *staffHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, 503, "staff_unavailable", "staff requires a database")
	return false
}
func staffError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, staff.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid staff input")
	case errors.Is(err, staff.ErrNotFound):
		failure(c, 404, "not_found", "staff record not found")
	case errors.Is(err, staff.ErrDuplicate):
		failure(c, 409, "duplicate_staff_record", "staff record already exists")
	case errors.Is(err, staff.ErrConflict):
		failure(c, 409, "staff_conflict", "staff relationship or state conflict")
	case errors.Is(err, staff.ErrForbidden):
		failure(c, 403, "forbidden", "permission denied")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}
func staffActor(c *gin.Context) string { return c.MustGet("auth_user").(auth.User).ID }

// listEmployees godoc
// @Summary Search and paginate the employee directory
// @Tags staff
// @Router /staff/employees [get]
func (h *staffHandler) listEmployees(c *gin.Context) {
	if !h.available(c) {
		return
	}
	page, size, err := parsePage(c)
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	v, err := h.service.ListEmployees(c.Request.Context(), staff.Query{Search: c.Query("search"), Kind: c.Query("kind"), Status: c.Query("status"), Page: page, PageSize: size})
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// getEmployee godoc
// @Summary Read an employee profile
// @Tags staff
// @Router /staff/employees/{id} [get]
func (h *staffHandler) getEmployee(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.GetEmployee(c.Request.Context(), c.Param("id"))
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// saveEmployee godoc
// @Summary Create or update an employee profile
// @Tags staff
// @Router /staff/employees [post]
func (h *staffHandler) saveEmployee(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input staff.EmployeeInput
	if !decodeSchoolJSON(c, 16*1024, &input) {
		return
	}
	v, err := h.service.SaveEmployee(c.Request.Context(), staffActor(c), c.Param("id"), input)
	if err != nil {
		staffError(c, err)
		return
	}
	status := 200
	if c.Request.Method == http.MethodPost {
		status = 201
	}
	success(c, status, v)
}

// ownEmployee godoc
// @Summary Read the signed-in employee profile
// @Tags staff
// @Router /staff/me [get]
func (h *staffHandler) ownEmployee(c *gin.Context) {
	if !h.available(c) {
		return
	}
	v, err := h.service.OwnEmployee(c.Request.Context(), staffActor(c))
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// listAssignments godoc
// @Summary Read teacher subject and class assignments
// @Tags staff
// @Router /staff/employees/{id}/assignments [get]
func (h *staffHandler) listAssignments(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := staffSubQuery(c)
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	v, err := h.service.ListAssignments(c.Request.Context(), c.Param("id"), query)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *staffHandler) ownAssignments(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := staffSubQuery(c)
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	e, err := h.service.OwnEmployee(c.Request.Context(), staffActor(c))
	if err != nil {
		staffError(c, err)
		return
	}
	v, err := h.service.ListAssignments(c.Request.Context(), e.ID, query)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// addAssignment godoc
// @Summary Assign a teacher to a session class subject pair
// @Tags staff
// @Router /staff/employees/{id}/assignments [post]
func (h *staffHandler) addAssignment(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input staff.AssignmentInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	v, err := h.service.AddAssignment(c.Request.Context(), staffActor(c), c.Param("id"), input)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 201, v)
}

// deleteAssignment godoc
// @Summary Remove a teacher assignment
// @Tags staff
// @Router /staff/employees/{id}/assignments/{assignmentId} [delete]
func (h *staffHandler) deleteAssignment(c *gin.Context) {
	if !h.available(c) {
		return
	}
	err := h.service.DeleteAssignment(c.Request.Context(), staffActor(c), c.Param("id"), c.Param("assignmentId"))
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, gin.H{"deleted": true})
}

// listLeaves godoc
// @Summary Read an employee's leave history
// @Tags staff
// @Router /staff/employees/{id}/leaves [get]
func (h *staffHandler) listLeaves(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := staffSubQuery(c)
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	v, err := h.service.ListLeaves(c.Request.Context(), staffActor(c), c.Param("id"), false, query)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *staffHandler) ownLeaves(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := staffSubQuery(c)
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	v, err := h.service.ListLeaves(c.Request.Context(), staffActor(c), "", true, query)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// requestLeave godoc
// @Summary Request employee leave
// @Tags staff
// @Router /staff/me/leaves [post]
func (h *staffHandler) requestLeave(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input staff.LeaveInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	self := c.Param("id") == ""
	v, err := h.service.RequestLeave(c.Request.Context(), staffActor(c), c.Param("id"), self, input)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 201, v)
}

// decideLeave godoc
// @Summary Approve or reject a pending leave request
// @Tags staff
// @Router /staff/leaves/{id}/decision [patch]
func (h *staffHandler) decideLeave(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if !decodeSchoolJSON(c, 1024, &input) {
		return
	}
	v, err := h.service.DecideLeave(c.Request.Context(), staffActor(c), c.Param("id"), input.Status)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// listSalaries godoc
// @Summary Read private employee salary history
// @Tags staff
// @Router /staff/employees/{id}/salaries [get]
func (h *staffHandler) listSalaries(c *gin.Context) {
	if !h.available(c) {
		return
	}
	query, err := staffSubQuery(c)
	if err != nil {
		staffError(c, staff.ErrInvalid)
		return
	}
	v, err := h.service.ListSalaries(c.Request.Context(), c.Param("id"), query)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 200, v)
}

// addSalary godoc
// @Summary Add a private salary record
// @Tags staff
// @Router /staff/employees/{id}/salaries [post]
func (h *staffHandler) addSalary(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input staff.SalaryInput
	if !decodeSchoolJSON(c, 8*1024, &input) {
		return
	}
	v, err := h.service.AddSalary(c.Request.Context(), staffActor(c), c.Param("id"), input)
	if err != nil {
		staffError(c, err)
		return
	}
	success(c, 201, v)
}
