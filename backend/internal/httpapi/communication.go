package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/diary"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notices"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notify"
)

// Communication groups the M9 services: notices and events, notifications,
// the class diary and attachment storage.
type Communication struct {
	Notices *notices.Service
	Notify  *notify.Service
	Diary   *diary.Service
	Files   *files.Service
}

type commHandler struct {
	auth *auth.Service
	svc  *Communication
}

// RequireAnyPermission passes when the user holds at least one permission.
// Record-level rules in the service then decide what that user may see.
func RequireAnyPermission(service *auth.Service, permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("auth_user")
		current, valid := user.(auth.User)
		if !ok || !valid || service == nil {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		for _, p := range permissions {
			allowed, err := service.HasPermission(c.Request.Context(), current.ID, p)
			if err != nil {
				failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
				c.Abort()
				return
			}
			if allowed {
				c.Next()
				return
			}
		}
		failure(c, http.StatusForbidden, "forbidden", "permission denied")
		c.Abort()
	}
}

func (h *commHandler) ready(c *gin.Context) bool {
	if h.svc != nil && h.svc.Notices != nil && h.svc.Notify != nil && h.svc.Diary != nil && h.svc.Files != nil {
		return true
	}
	failure(c, 503, "communication_unavailable", "notices, notifications and diary require a database")
	return false
}

func commUser(c *gin.Context) auth.User { return c.MustGet("auth_user").(auth.User) }

func (h *commHandler) has(c *gin.Context, permission string) bool {
	ok, err := h.auth.HasPermission(c.Request.Context(), commUser(c).ID, permission)
	return err == nil && ok
}

func commError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, notices.ErrInvalid), errors.Is(err, notify.ErrInvalid), errors.Is(err, diary.ErrInvalid), errors.Is(err, files.ErrInvalid):
		failure(c, 400, "invalid_request", "invalid input")
	case errors.Is(err, files.ErrType):
		failure(c, 415, "unsupported_file", "only PDF, JPEG and PNG files are accepted")
	case errors.Is(err, files.ErrTooLarge):
		failure(c, 413, "file_too_large", "file is too large or too many uploads are pending")
	case errors.Is(err, notices.ErrNotFound), errors.Is(err, notify.ErrNotFound), errors.Is(err, diary.ErrNotFound), errors.Is(err, files.ErrNotFound):
		failure(c, 404, "not_found", "not found")
	case errors.Is(err, diary.ErrClosed):
		failure(c, 409, "submission_closed", "homework submission is not open")
	case errors.Is(err, notices.ErrConflict), errors.Is(err, diary.ErrConflict):
		failure(c, 409, "conflict", "the record changed state or already exists")
	case errors.Is(err, notices.ErrForbidden), errors.Is(err, diary.ErrForbidden):
		failure(c, 403, "forbidden", "not assigned to this class and subject")
	default:
		failure(c, 500, "internal_error", "internal server error")
	}
}

func reply[T any](c *gin.Context, status int, value T, err error) {
	if err != nil {
		commError(c, err)
		return
	}
	success(c, status, value)
}

func pageQuery(c *gin.Context) (int, int, bool) {
	page, size, err := parsePage(c)
	if c.Query("page_size") == "" {
		size = 20
	}
	if err != nil {
		commError(c, notices.ErrInvalid)
		return 0, 0, false
	}
	return page, size, true
}

// upload accepts one multipart "file" field for the given purpose.
func (h *commHandler) upload(purpose string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.ready(c) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, files.MaxPDFBytes+64*1024)
		header, err := c.FormFile("file")
		if err != nil {
			commError(c, files.ErrTooLarge)
			return
		}
		f, err := header.Open()
		if err != nil {
			commError(c, files.ErrInvalid)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, files.MaxPDFBytes+1))
		if err != nil {
			commError(c, files.ErrInvalid)
			return
		}
		meta, err := h.svc.Files.Upload(c.Request.Context(), commUser(c).ID, purpose, header.Filename, data)
		reply(c, http.StatusCreated, meta, err)
	}
}

// serve streams an authorized file with headers that stop browsers from
// sniffing or running it; ?download=1 forces a download.
func (h *commHandler) serve(c *gin.Context, fileID string) {
	meta, body, err := h.svc.Files.Open(c.Request.Context(), fileID)
	if err != nil {
		commError(c, err)
		return
	}
	defer body.Close()
	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}
	c.Header("Content-Type", meta.ContentType)
	c.Header("Content-Length", strconv.Itoa(meta.Size))
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": meta.Name}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, body)
}

// --- Notices ---

func (h *commHandler) noticeQuery(c *gin.Context) (notices.Query, bool) {
	page, size, ok := pageQuery(c)
	return notices.Query{Status: c.Query("status"), Search: c.Query("search"), Priority: c.Query("priority"), From: c.Query("from"), To: c.Query("to"), Page: page, PageSize: size}, ok
}
func (h *commHandler) noticeFeed(c *gin.Context) {
	if q, ok := h.noticeQuery(c); ok && h.ready(c) {
		v, err := h.svc.Notices.Feed(c.Request.Context(), commUser(c).ID, q)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) noticesManaged(c *gin.Context) {
	if q, ok := h.noticeQuery(c); ok && h.ready(c) {
		v, err := h.svc.Notices.ListManaged(c.Request.Context(), q)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) notice(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Notices.Get(c.Request.Context(), commUser(c).ID, c.Param("id"), h.has(c, "notices:manage"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) saveNotice(c *gin.Context) {
	var in notices.NoticeInput
	if h.ready(c) && decodeSchoolJSON(c, 160*1024, &in) {
		v, err := h.svc.Notices.Save(c.Request.Context(), commUser(c).ID, c.Param("id"), in)
		reply(c, created(c), v, err)
	}
}
func (h *commHandler) publishNotice(c *gin.Context) {
	var in notices.PublishInput
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.svc.Notices.Publish(c.Request.Context(), commUser(c).ID, c.Param("id"), in)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) archiveNotice(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Notices.Archive(c.Request.Context(), commUser(c).ID, c.Param("id"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) deleteNotice(c *gin.Context) {
	if h.ready(c) {
		reply(c, 200, gin.H{"deleted": true}, h.svc.Notices.Delete(c.Request.Context(), commUser(c).ID, c.Param("id")))
	}
}
func (h *commHandler) noticeFile(c *gin.Context) {
	if h.ready(c) {
		id, err := h.svc.Notices.AttachmentFile(c.Request.Context(), commUser(c).ID, c.Param("id"), c.Param("fileId"), h.has(c, "notices:manage"))
		if err != nil {
			commError(c, err)
			return
		}
		h.serve(c, id)
	}
}

// --- Events ---

func (h *commHandler) events(c *gin.Context) {
	if h.ready(c) {
		manager := c.Query("manage") == "1" && h.has(c, "events:manage")
		v, err := h.svc.Notices.ListEvents(c.Request.Context(), commUser(c).ID, notices.Query{From: c.Query("from"), To: c.Query("to"), Status: c.Query("status")}, manager)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) saveEvent(c *gin.Context) {
	var in notices.EventInput
	if h.ready(c) && decodeSchoolJSON(c, 64*1024, &in) {
		v, err := h.svc.Notices.SaveEvent(c.Request.Context(), commUser(c).ID, c.Param("id"), in)
		reply(c, created(c), v, err)
	}
}
func (h *commHandler) eventStatus(status string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.ready(c) {
			v, err := h.svc.Notices.SetEventStatus(c.Request.Context(), commUser(c).ID, c.Param("id"), status)
			reply(c, 200, v, err)
		}
	}
}
func (h *commHandler) deleteEvent(c *gin.Context) {
	if h.ready(c) {
		reply(c, 200, gin.H{"deleted": true}, h.svc.Notices.DeleteEvent(c.Request.Context(), commUser(c).ID, c.Param("id")))
	}
}

// --- Notifications ---

func (h *commHandler) inbox(c *gin.Context) {
	if page, size, ok := pageQuery(c); ok && h.ready(c) {
		v, err := h.svc.Notify.Inbox(c.Request.Context(), commUser(c).ID, c.Query("unread") == "1", page, size)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) markRead(c *gin.Context) {
	if h.ready(c) {
		reply(c, 200, gin.H{"read": true}, h.svc.Notify.MarkRead(c.Request.Context(), commUser(c).ID, c.Param("id")))
	}
}
func (h *commHandler) markAllRead(c *gin.Context) {
	if h.ready(c) {
		n, err := h.svc.Notify.MarkAllRead(c.Request.Context(), commUser(c).ID)
		reply(c, 200, gin.H{"read": n}, err)
	}
}
func (h *commHandler) preferences(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Notify.Preferences(c.Request.Context(), commUser(c).ID)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) savePreferences(c *gin.Context) {
	var in struct {
		Items []notify.Preference `json:"items"`
	}
	if h.ready(c) && decodeSchoolJSON(c, 4*1024, &in) {
		v, err := h.svc.Notify.SavePreferences(c.Request.Context(), commUser(c).ID, in.Items)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) dispatch(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Notify.Dispatch(c.Request.Context())
		reply(c, 200, v, err)
	}
}
func (h *commHandler) smsLog(c *gin.Context) {
	if page, size, ok := pageQuery(c); ok && h.ready(c) {
		v, err := h.svc.Notify.SMSLog(c.Request.Context(), c.Query("status"), page, size)
		reply(c, 200, v, err)
	}
}

// --- Diary ---

func (h *commHandler) viewer(c *gin.Context, studentID string) diary.Viewer {
	u := commUser(c)
	return diary.Viewer{UserID: u.ID, Role: u.Role, Oversight: h.has(c, "diary:read"), StudentID: studentID}
}
func (h *commHandler) diaryQuery(c *gin.Context) (diary.Query, bool) {
	page, size, ok := pageQuery(c)
	return diary.Query{From: c.Query("from"), To: c.Query("to"), SubjectID: c.Query("subject_id"), SectionID: c.Query("section_id"), Status: c.Query("status"), Search: c.Query("search"), Page: page, PageSize: size}, ok
}
func (h *commHandler) diaryAssignments(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.Assignments(c.Request.Context(), commUser(c).ID, c.Query("date"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) diaryTeaching(c *gin.Context) {
	if q, ok := h.diaryQuery(c); ok && h.ready(c) {
		v, err := h.svc.Diary.Teaching(c.Request.Context(), commUser(c).ID, q)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) diaryFeed(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if commUser(c).Role != role {
			commError(c, diary.ErrForbidden)
			return
		}
		if q, ok := h.diaryQuery(c); ok && h.ready(c) {
			v, err := h.svc.Diary.Feed(c.Request.Context(), h.viewer(c, c.Param("studentId")), q)
			reply(c, 200, v, err)
		}
	}
}
func (h *commHandler) diaryOversight(c *gin.Context) {
	if q, ok := h.diaryQuery(c); ok && h.ready(c) {
		v, err := h.svc.Diary.Oversight(c.Request.Context(), q)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) diaryGet(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.Get(c.Request.Context(), h.viewer(c, c.Query("student_id")), c.Param("id"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) diarySave(c *gin.Context) {
	var in diary.DiaryInput
	if h.ready(c) && decodeSchoolJSON(c, 96*1024, &in) {
		v, err := h.svc.Diary.Save(c.Request.Context(), commUser(c).ID, c.Param("id"), in)
		reply(c, created(c), v, err)
	}
}
func (h *commHandler) diaryPublish(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.Publish(c.Request.Context(), commUser(c).ID, c.Param("id"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) diaryDelete(c *gin.Context) {
	if h.ready(c) {
		reply(c, 200, gin.H{"deleted": true}, h.svc.Diary.Delete(c.Request.Context(), commUser(c).ID, c.Param("id")))
	}
}
func (h *commHandler) diaryFile(c *gin.Context) {
	if h.ready(c) {
		if err := h.svc.Diary.FileAccess(c.Request.Context(), h.viewer(c, c.Query("student_id")), c.Param("id"), c.Param("fileId")); err != nil {
			commError(c, err)
			return
		}
		h.serve(c, c.Param("fileId"))
	}
}
func (h *commHandler) mySubmission(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.MySubmission(c.Request.Context(), commUser(c).ID, c.Param("id"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) submit(c *gin.Context) {
	if commUser(c).Role != "student" {
		commError(c, diary.ErrForbidden)
		return
	}
	var in diary.SubmissionInput
	if h.ready(c) && decodeSchoolJSON(c, 32*1024, &in) {
		v, err := h.svc.Diary.Submit(c.Request.Context(), commUser(c).ID, c.Param("id"), in)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) submissions(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.Submissions(c.Request.Context(), h.viewer(c, ""), c.Param("id"))
		reply(c, 200, v, err)
	}
}
func (h *commHandler) review(c *gin.Context) {
	var in struct {
		Note string `json:"note"`
	}
	if h.ready(c) && decodeSchoolJSON(c, 8*1024, &in) {
		v, err := h.svc.Diary.Review(c.Request.Context(), commUser(c).ID, c.Param("id"), in.Note)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) submissionFile(c *gin.Context) {
	if h.ready(c) {
		if err := h.svc.Diary.SubmissionFileAccess(c.Request.Context(), h.viewer(c, ""), c.Param("id"), c.Param("fileId")); err != nil {
			commError(c, err)
			return
		}
		h.serve(c, c.Param("fileId"))
	}
}
func (h *commHandler) diarySettings(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.Settings(c.Request.Context())
		reply(c, 200, v, err)
	}
}
func (h *commHandler) saveDiarySettings(c *gin.Context) {
	var in diary.Settings
	if h.ready(c) && decodeSchoolJSON(c, 1024, &in) {
		v, err := h.svc.Diary.SaveSettings(c.Request.Context(), commUser(c).ID, in)
		reply(c, 200, v, err)
	}
}
func (h *commHandler) compliance(c *gin.Context) {
	if h.ready(c) {
		v, err := h.svc.Diary.Compliance(c.Request.Context(), c.Query("date"))
		reply(c, 200, v, err)
	}
}

func registerCommunication(g *gin.RouterGroup, service *auth.Service, comm *Communication, moduleGuard func(string) gin.HandlerFunc) {
	h := &commHandler{auth: service, svc: comm}
	perm := func(p string) gin.HandlerFunc { return RequirePermission(service, p) }
	notice, diaryOn := moduleGuard("notices"), moduleGuard("diary")
	managers := RequireAnyRole("admin", "head_teacher")
	writers := RequireAnyRole("teacher", "head_teacher")

	g.GET("/notices", perm("notices:read"), notice, h.noticeFeed)
	g.GET("/notices/manage", perm("notices:manage"), notice, h.noticesManaged)
	g.POST("/notices", perm("notices:manage"), notice, managers, h.saveNotice)
	g.POST("/notices/files", perm("notices:manage"), notice, managers, h.upload("notice"))
	g.GET("/notices/:id", perm("notices:read"), notice, h.notice)
	g.PATCH("/notices/:id", perm("notices:manage"), notice, managers, h.saveNotice)
	g.DELETE("/notices/:id", perm("notices:manage"), notice, managers, h.deleteNotice)
	g.POST("/notices/:id/publish", perm("notices:manage"), notice, managers, h.publishNotice)
	g.POST("/notices/:id/archive", perm("notices:manage"), notice, managers, h.archiveNotice)
	g.GET("/notices/:id/files/:fileId", perm("notices:read"), notice, h.noticeFile)

	g.GET("/events", perm("events:read"), notice, h.events)
	g.POST("/events", perm("events:manage"), notice, managers, h.saveEvent)
	g.PATCH("/events/:id", perm("events:manage"), notice, managers, h.saveEvent)
	g.DELETE("/events/:id", perm("events:manage"), notice, managers, h.deleteEvent)
	g.POST("/events/:id/publish", perm("events:manage"), notice, managers, h.eventStatus("published"))
	g.POST("/events/:id/cancel", perm("events:manage"), notice, managers, h.eventStatus("cancelled"))

	g.GET("/notifications", perm("notifications:read_self"), notice, h.inbox)
	g.POST("/notifications/read-all", perm("notifications:read_self"), notice, h.markAllRead)
	g.GET("/notifications/preferences", perm("notifications:read_self"), notice, h.preferences)
	g.PUT("/notifications/preferences", perm("notifications:read_self"), notice, h.savePreferences)
	g.POST("/notifications/dispatch", perm("notifications:manage"), notice, RequireAdministrator(), h.dispatch)
	g.GET("/notifications/sms", perm("notifications:manage"), notice, RequireAdministrator(), h.smsLog)
	g.POST("/notifications/:id/read", perm("notifications:read_self"), notice, h.markRead)

	anyDiary := RequireAnyPermission(service, "diary:write", "diary:read", "diary:read_self", "diary:read_children")
	g.GET("/diary/assignments", perm("diary:write"), diaryOn, h.diaryAssignments)
	g.GET("/diary/teaching", perm("diary:write"), diaryOn, h.diaryTeaching)
	g.POST("/diary", perm("diary:write"), diaryOn, writers, h.diarySave)
	g.POST("/diary/files", perm("diary:write"), diaryOn, writers, h.upload("diary"))
	g.GET("/diary/me", perm("diary:read_self"), diaryOn, h.diaryFeed("student"))
	g.GET("/diary/children/:studentId", perm("diary:read_children"), diaryOn, h.diaryFeed("guardian"))
	g.GET("/diary/all", perm("diary:read"), diaryOn, h.diaryOversight)
	g.GET("/diary/compliance", perm("diary:read"), diaryOn, h.compliance)
	g.GET("/diary/settings", perm("diary:manage"), diaryOn, h.diarySettings)
	g.PUT("/diary/settings", perm("diary:manage"), diaryOn, RequireAdministrator(), h.saveDiarySettings)
	g.POST("/diary/homework/files", perm("diary:read_self"), diaryOn, RequireAnyRole("student"), h.upload("homework"))
	g.PATCH("/diary/submissions/:id/review", perm("diary:write"), diaryOn, writers, h.review)
	g.GET("/diary/submissions/:id/files/:fileId", anyDiary, diaryOn, h.submissionFile)
	g.GET("/diary/:id", anyDiary, diaryOn, h.diaryGet)
	g.PATCH("/diary/:id", perm("diary:write"), diaryOn, writers, h.diarySave)
	g.DELETE("/diary/:id", perm("diary:write"), diaryOn, writers, h.diaryDelete)
	g.POST("/diary/:id/publish", perm("diary:write"), diaryOn, writers, h.diaryPublish)
	g.GET("/diary/:id/files/:fileId", anyDiary, diaryOn, h.diaryFile)
	g.GET("/diary/:id/submissions", RequireAnyPermission(service, "diary:write", "diary:read"), diaryOn, h.submissions)
	g.GET("/diary/:id/submission", perm("diary:read_self"), diaryOn, h.mySubmission)
	g.PUT("/diary/:id/submission", perm("diary:read_self"), diaryOn, h.submit)
}
