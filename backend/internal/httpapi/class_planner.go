package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
)

func (h *routineHandler) plannerReady(c *gin.Context) bool {
	if !h.available(c) {
		return false
	}
	if h.service.ClassPlanner != nil {
		return true
	}
	failure(c, 503, "routine_unavailable", "class planner requires a database")
	return false
}
func (h *routineHandler) classPlannerPreview(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	var in routine.ClassPlan
	if !decodeSchoolJSON(c, 512*1024, &in) {
		return
	}
	v, err := h.service.ClassPlanner.Preview(c.Request.Context(), in)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *routineHandler) classPlannerDraft(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	v, err := h.service.ClassPlanner.Draft(c.Request.Context(), c.Query("session_id"))
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *routineHandler) classPlannerSave(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	var in struct {
		Plan          routine.ClassPlan `json:"plan"`
		PreviewDigest string            `json:"preview_digest"`
	}
	if !decodeSchoolJSON(c, 512*1024, &in) {
		return
	}
	v, err := h.service.ClassPlanner.SaveDraft(c.Request.Context(), routineUser(c).ID, in.Plan, in.PreviewDigest)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}
func (h *routineHandler) classPlannerPublish(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	var in struct {
		SessionID     string `json:"session_id"`
		DraftID       string `json:"draft_id"`
		PreviewDigest string `json:"preview_digest"`
		Confirmed     bool   `json:"confirmed"`
	}
	if !decodeSchoolJSON(c, 8*1024, &in) {
		return
	}
	v, err := h.service.ClassPlanner.Publish(c.Request.Context(), routineUser(c).ID, in.SessionID, in.DraftID, in.PreviewDigest, in.Confirmed)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *routineHandler) cohortLessons(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	u := routineUser(c)
	v, err := h.service.ClassPlanner.CohortLessons(c.Request.Context(), u.ID, u.Role, c.Query("session_id"))
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *routineHandler) cohortRoster(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	u := routineUser(c)
	v, err := h.service.ClassPlanner.CohortRoster(c.Request.Context(), u.ID, u.Role, c.Param("id"), c.Query("date"))
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}

func (h *routineHandler) cohortAttendance(c *gin.Context) {
	if !h.plannerReady(c) {
		return
	}
	var in struct {
		Date    string                          `json:"date"`
		Entries []routine.CohortAttendanceEntry `json:"entries"`
	}
	if !decodeSchoolJSON(c, 128*1024, &in) {
		return
	}
	u := routineUser(c)
	v, err := h.service.ClassPlanner.SaveCohortAttendance(c.Request.Context(), u.ID, u.Role, c.Param("id"), in.Date, in.Entries)
	if err != nil {
		routineError(c, err)
		return
	}
	success(c, 200, v)
}
