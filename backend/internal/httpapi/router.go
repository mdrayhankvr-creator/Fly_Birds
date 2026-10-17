package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academicsessions"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
)

func NewRouter() *gin.Engine {
	return NewRouterWithAuth(nil, true)
}

func NewRouterWithAuth(service *auth.Service, secureCookies bool, authorization ...*rbac.Service) *gin.Engine {
	return NewRouterWithServices(service, secureCookies, firstRBAC(authorization), nil)
}

func firstRBAC(services []*rbac.Service) *rbac.Service {
	if len(services) > 0 {
		return services[0]
	}
	return nil
}

func NewRouterWithServices(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service) *gin.Engine {
	return NewRouterWithAll(service, secureCookies, grants, schoolService, nil, nil, nil)
}

func NewRouterWithAll(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionServices ...*academicsessions.Service) *gin.Engine {
	var sessionService *academicsessions.Service
	if len(sessionServices) > 0 {
		sessionService = sessionServices[0]
	}
	return NewRouterWithAcademicServices(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, nil)
}

func NewRouterWithAcademicServices(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, extraServices ...any) *gin.Engine {
	var sectionService *academic.SectionService
	var staffService *staff.Service
	for _, extra := range extraServices {
		switch typed := extra.(type) {
		case *academic.SectionService:
			sectionService = typed
		case *staff.Service:
			staffService = typed
		}
	}
	return NewRouterWithIntegratedServices(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, academicService, sectionService, nil, staffService)
}

func NewRouterWithPeople(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, sectionService *academic.SectionService, peopleService *people.Service) *gin.Engine {
	return NewRouterWithIntegratedServices(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, academicService, sectionService, peopleService, nil)
}

func NewRouterWithBatchServices(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, staffService *staff.Service, attendanceService *attendance.Service) *gin.Engine {
	return NewRouterWithIntegratedServices(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, academicService, nil, nil, staffService, attendanceService)
}

func NewRouterWithIntegratedServices(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, sectionService *academic.SectionService, peopleService *people.Service, staffService *staff.Service, attendanceServices ...*attendance.Service) *gin.Engine {
	var attendanceService *attendance.Service
	if len(attendanceServices) > 0 {
		attendanceService = attendanceServices[0]
	}
	return NewRouterWithStudentAttendance(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, academicService, sectionService, peopleService, staffService, attendanceService, nil)
}

func NewRouterWithStudentAttendance(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, sectionService *academic.SectionService, peopleService *people.Service, staffService *staff.Service, attendanceService *attendance.Service, studentAttendanceService *attendance.StudentService) *gin.Engine {
	return NewRouterWithRoutine(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, academicService, sectionService, peopleService, staffService, attendanceService, studentAttendanceService, nil)
}

func NewRouterWithRoutine(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, sectionService *academic.SectionService, peopleService *people.Service, staffService *staff.Service, attendanceService *attendance.Service, studentAttendanceService *attendance.StudentService, routineService *routine.Service) *gin.Engine {
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.Logger(), gin.CustomRecovery(func(c *gin.Context, _ any) {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		c.Abort()
	}))
	router.HandleMethodNotAllowed = true
	moduleGuard := func(key string) gin.HandlerFunc {
		if schoolService == nil { // Compatibility for health-only and isolated legacy router tests.
			return func(c *gin.Context) { c.Next() }
		}
		return RequireModule(schoolService, key)
	}

	router.GET("/healthz", healthz)
	handler := &authHandler{service: service, secureCookies: secureCookies}
	authRoutes := router.Group("/auth")
	authRoutes.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") })
	authRoutes.POST("/login", handler.login)
	authRoutes.POST("/refresh", handler.refresh)
	authRoutes.POST("/logout", handler.logout)
	authRoutes.GET("/me", RequireAuth(service), RequirePermission(service, "auth:read_self"), moduleGuard("authentication"), handler.me)
	authRoutes.GET("/permissions", RequireAuth(service), RequirePermission(service, "auth:read_self"), moduleGuard("permissions"), handler.permissions)
	rbacHandler := &rbacHandler{service: grants}
	management := router.Group("")
	management.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service))
	management.GET("/roles", RequirePermission(service, "roles:read"), moduleGuard("permissions"), rbacHandler.roles)
	management.GET("/permissions", RequirePermission(service, "permissions:read"), moduleGuard("permissions"), rbacHandler.permissions)
	management.GET("/roles/:id/permissions", RequirePermission(service, "permissions:read"), moduleGuard("permissions"), rbacHandler.rolePermissions)
	management.PUT("/roles/:id/permissions", RequirePermission(service, "permissions:manage"), moduleGuard("permissions"), RequireAdministrator(), rbacHandler.setRolePermissions)
	management.GET("/users/:id/roles", RequirePermission(service, "roles:read"), moduleGuard("permissions"), rbacHandler.userRoles)
	management.PUT("/users/:id/roles", RequirePermission(service, "permissions:manage"), moduleGuard("permissions"), RequireAdministrator(), rbacHandler.setUserRoles)
	usersHandler := &usersHandler{service: userService}
	management.GET("/users", RequirePermission(service, "users:read"), moduleGuard("permissions"), RequireAdministrator(), usersHandler.list)
	management.POST("/users", RequirePermission(service, "users:create"), moduleGuard("permissions"), RequireAdministrator(), usersHandler.create)
	management.GET("/users/:id", RequirePermission(service, "users:read"), moduleGuard("permissions"), RequireAdministrator(), usersHandler.get)
	management.PATCH("/users/:id", RequirePermission(service, "users:update"), moduleGuard("permissions"), RequireAdministrator(), usersHandler.update)
	management.PATCH("/users/:id/status", RequirePermission(service, "users:update"), moduleGuard("permissions"), RequireAdministrator(), usersHandler.status)
	management.GET("/audit/events", RequirePermission(service, "audit:read"), moduleGuard("permissions"), RequireAdministrator(), (&auditHandler{service: auditService}).events)
	management.GET("/dashboard", RequirePermission(service, "auth:read_self"), moduleGuard("authentication"), (&dashboardHandler{service: dashboardService}).get)
	sessionsHandler := &academicSessionsHandler{service: sessionService}
	management.GET("/academic/sessions", RequirePermission(service, "academic_sessions:read"), moduleGuard("academics"), sessionsHandler.list)
	management.GET("/academic/sessions/current", RequirePermission(service, "academic_sessions:read"), moduleGuard("academics"), sessionsHandler.current)
	management.GET("/academic/sessions/:id", RequirePermission(service, "academic_sessions:read"), moduleGuard("academics"), sessionsHandler.get)
	management.POST("/academic/sessions", RequirePermission(service, "academic_sessions:create"), moduleGuard("academics"), RequireAdministrator(), sessionsHandler.create)
	management.PATCH("/academic/sessions/:id", RequirePermission(service, "academic_sessions:update"), moduleGuard("academics"), RequireAdministrator(), sessionsHandler.update)
	management.PATCH("/academic/sessions/:id/activation", RequirePermission(service, "academic_sessions:activate"), moduleGuard("academics"), RequireAdministrator(), sessionsHandler.activation)
	staffHandler := &staffHandler{service: staffService}
	management.GET("/staff/employees", RequirePermission(service, "staff:read"), moduleGuard("staff"), staffHandler.listEmployees)
	management.POST("/staff/employees", RequirePermission(service, "staff:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.saveEmployee)
	management.GET("/staff/me", RequirePermission(service, "staff:read_self"), moduleGuard("staff"), staffHandler.ownEmployee)
	management.GET("/staff/me/assignments", RequirePermission(service, "staff:read_self"), moduleGuard("staff"), staffHandler.ownAssignments)
	management.GET("/staff/me/leaves", RequirePermission(service, "staff_leaves:request_self"), moduleGuard("staff"), staffHandler.ownLeaves)
	management.POST("/staff/me/leaves", RequirePermission(service, "staff_leaves:request_self"), moduleGuard("staff"), staffHandler.requestLeave)
	management.GET("/staff/employees/:id", RequirePermission(service, "staff:read"), moduleGuard("staff"), staffHandler.getEmployee)
	management.PATCH("/staff/employees/:id", RequirePermission(service, "staff:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.saveEmployee)
	management.GET("/staff/employees/:id/assignments", RequirePermission(service, "staff:read"), moduleGuard("staff"), staffHandler.listAssignments)
	management.POST("/staff/employees/:id/assignments", RequirePermission(service, "staff:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.addAssignment)
	management.DELETE("/staff/employees/:id/assignments/:assignmentId", RequirePermission(service, "staff:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.deleteAssignment)
	management.GET("/staff/employees/:id/leaves", RequirePermission(service, "staff_leaves:read"), moduleGuard("staff"), staffHandler.listLeaves)
	management.POST("/staff/employees/:id/leaves", RequirePermission(service, "staff_leaves:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.requestLeave)
	management.PATCH("/staff/leaves/:id/decision", RequirePermission(service, "staff_leaves:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.decideLeave)
	management.GET("/staff/employees/:id/salaries", RequirePermission(service, "staff_salary:read"), moduleGuard("staff"), RequireAdministrator(), staffHandler.listSalaries)
	management.POST("/staff/employees/:id/salaries", RequirePermission(service, "staff_salary:manage"), moduleGuard("staff"), RequireAdministrator(), staffHandler.addSalary)
	attendanceHandler := &attendanceHandler{service: attendanceService}
	management.GET("/attendance/sessions", RequirePermission(service, "attendance:teachers_read_self"), moduleGuard("attendance"), attendanceHandler.attendanceSessions)
	management.GET("/attendance/teachers/roster", RequirePermission(service, "attendance:teachers_read"), moduleGuard("attendance"), attendanceHandler.teacherRoster)
	management.POST("/attendance/teachers", RequirePermission(service, "attendance:teachers_manage"), moduleGuard("attendance"), attendanceHandler.recordTeacher)
	management.PATCH("/attendance/teachers/:id", RequirePermission(service, "attendance:teachers_manage"), moduleGuard("attendance"), attendanceHandler.correctTeacher)
	management.GET("/attendance/teachers/me", RequirePermission(service, "attendance:teachers_read_self"), moduleGuard("attendance"), attendanceHandler.ownTeacherHistory)
	management.GET("/attendance/teachers/monthly", RequirePermission(service, "attendance:teachers_reports"), moduleGuard("attendance"), attendanceHandler.teacherMonthly)
	management.GET("/attendance/teachers/me/monthly", RequirePermission(service, "attendance:teachers_read_self"), moduleGuard("attendance"), attendanceHandler.ownTeacherMonthly)
	studentAttendance := &studentAttendanceHandler{service: studentAttendanceService}
	management.GET("/attendance/students/sections", RequirePermission(service, "attendance:students_read"), moduleGuard("attendance"), studentAttendance.sections)
	management.GET("/attendance/students/roster", RequirePermission(service, "attendance:students_read"), moduleGuard("attendance"), studentAttendance.roster)
	management.POST("/attendance/students", RequirePermission(service, "attendance:students_manage"), moduleGuard("attendance"), studentAttendance.record)
	management.POST("/attendance/students/qr", RequirePermission(service, "attendance:students_manage"), moduleGuard("attendance"), studentAttendance.scan)
	management.PATCH("/attendance/students/:id", RequirePermission(service, "attendance:students_manage"), moduleGuard("attendance"), studentAttendance.correct)
	management.GET("/attendance/students/monthly", RequirePermission(service, "attendance:students_read"), moduleGuard("attendance"), studentAttendance.monthly)
	management.GET("/attendance/students/me/report", RequirePermission(service, "attendance:students_read_self"), moduleGuard("attendance"), studentAttendance.self)
	management.GET("/attendance/guardians/children/:id/report", RequirePermission(service, "attendance:guardians_read_children"), moduleGuard("attendance"), studentAttendance.child)
	management.GET("/attendance/alerts", RequirePermission(service, "attendance:alerts_read"), moduleGuard("attendance"), studentAttendance.alerts)
	tr := &routineHandler{service: routineService}
	management.GET("/routines/classes", RequirePermission(service, "routine:read"), moduleGuard("routine"), tr.classes)
	management.POST("/routines/classes", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.createClass)
	management.GET("/routines/classes/me", RequirePermission(service, "routine:read_self"), moduleGuard("routine"), tr.ownClasses)
	management.GET("/routines/classes/children/:studentId", RequirePermission(service, "routine:read_children"), moduleGuard("routine"), tr.childClasses)
	management.PATCH("/routines/classes/:id", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.updateClass)
	management.DELETE("/routines/classes/:id", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.deleteClass)
	management.GET("/routines/exams", RequirePermission(service, "routine:read"), moduleGuard("routine"), tr.exams)
	management.POST("/routines/exams", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.createExam)
	management.GET("/routines/exams/me", RequirePermission(service, "routine:read_self"), moduleGuard("routine"), tr.ownExams)
	management.GET("/routines/exams/me/:id", RequirePermission(service, "routine:read_self"), moduleGuard("routine"), tr.ownExam)
	management.GET("/routines/exams/children/:studentId", RequirePermission(service, "routine:read_children"), moduleGuard("routine"), tr.childExams)
	management.GET("/routines/exams/children/:studentId/:id", RequirePermission(service, "routine:read_children"), moduleGuard("routine"), tr.childExam)
	management.GET("/routines/exams/:id", RequirePermission(service, "routine:read"), moduleGuard("routine"), tr.exam)
	management.PATCH("/routines/exams/:id", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.updateExam)
	management.DELETE("/routines/exams/:id", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.deleteExam)
	management.PATCH("/routines/exams/:id/publication", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.publication)
	management.POST("/routines/exams/:id/slots", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.createExamSlot)
	management.PATCH("/routines/exams/:id/slots/:slotId", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.updateExamSlot)
	management.DELETE("/routines/exams/:id/slots/:slotId", RequirePermission(service, "routine:manage"), moduleGuard("routine"), RequireAdministrator(), tr.deleteExamSlot)
	schoolHandler := &schoolHandler{service: schoolService}
	router.GET("/school/branding", schoolHandler.branding)
	schoolRoutes := router.Group("/school")
	schoolRoutes.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service))
	schoolRoutes.GET("/settings", RequirePermission(service, "school_settings:read"), RequireModule(schoolService, "school_settings"), schoolHandler.settings)
	schoolRoutes.PUT("/settings", RequirePermission(service, "school_settings:update"), RequireModule(schoolService, "school_settings"), RequireAdministrator(), schoolHandler.saveSettings)
	schoolRoutes.GET("/grading", RequirePermission(service, "grading:read"), RequireModule(schoolService, "school_settings"), schoolHandler.grading)
	schoolRoutes.PUT("/grading", RequirePermission(service, "grading:update"), RequireModule(schoolService, "school_settings"), RequireAdministrator(), schoolHandler.saveGrading)
	schoolRoutes.GET("/modules", RequirePermission(service, "modules:read"), RequireModule(schoolService, "school_settings"), schoolHandler.modules)
	schoolRoutes.PUT("/modules/:key", RequirePermission(service, "modules:manage"), RequireModule(schoolService, "school_settings"), RequireAdministrator(), schoolHandler.saveModule)
	academicHandler := &academicHandler{service: academicService}
	academicRoutes := router.Group("/academic")
	academicRoutes.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service))
	academicRoutes.GET("/classes", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), academicHandler.classes)
	academicRoutes.POST("/classes", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.saveClass)
	academicRoutes.GET("/classes/:id", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), academicHandler.class)
	academicRoutes.PATCH("/classes/:id", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.saveClass)
	academicRoutes.GET("/subjects", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), academicHandler.subjects)
	academicRoutes.POST("/subjects", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.saveSubject)
	academicRoutes.GET("/subjects/:id", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), academicHandler.subject)
	academicRoutes.PATCH("/subjects/:id", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.saveSubject)
	academicRoutes.GET("/classes/:id/subjects", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), academicHandler.mappings)
	academicRoutes.POST("/classes/:id/subjects", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.createMapping)
	academicRoutes.PATCH("/classes/:id/subjects/:subjectId", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.updateMapping)
	academicRoutes.DELETE("/classes/:id/subjects/:subjectId", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), academicHandler.deleteMapping)
	sectionHandler := &sectionsHandler{service: sectionService}
	academicRoutes.GET("/sections", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), sectionHandler.list)
	academicRoutes.POST("/sections", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), sectionHandler.save)
	academicRoutes.GET("/sections/:id", RequirePermission(service, "academics:read"), RequireModule(schoolService, "academics"), sectionHandler.get)
	academicRoutes.PATCH("/sections/:id", RequirePermission(service, "academics:manage"), RequireModule(schoolService, "academics"), RequireAdministrator(), sectionHandler.save)
	peopleHandler := &peopleHandler{service: peopleService}
	studentRoutes := router.Group("/students")
	studentRoutes.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service), RequireModule(schoolService, "students"))
	studentRoutes.GET("", RequirePermission(service, "students:read"), peopleHandler.students)
	studentRoutes.POST("", RequirePermission(service, "students:manage"), RequireAdministrator(), peopleHandler.saveStudent)
	studentRoutes.POST("/import", RequirePermission(service, "students:manage"), RequireAdministrator(), peopleHandler.importStudents)
	studentRoutes.GET("/me", RequirePermission(service, "students:read_self"), peopleHandler.myStudent)
	studentRoutes.GET("/:id", RequirePermission(service, "auth:read_self"), peopleHandler.student)
	studentRoutes.PATCH("/:id", RequirePermission(service, "students:manage"), RequireAdministrator(), peopleHandler.saveStudent)
	studentRoutes.DELETE("/:id", RequirePermission(service, "students:manage"), RequireAdministrator(), peopleHandler.deleteStudent)
	studentRoutes.GET("/:id/card", RequirePermission(service, "auth:read_self"), peopleHandler.card)
	studentRoutes.GET("/:id/photo", RequirePermission(service, "auth:read_self"), peopleHandler.photo)
	studentRoutes.PUT("/:id/photo", RequirePermission(service, "students:manage"), RequireAdministrator(), peopleHandler.savePhoto)
	studentRoutes.GET("/:id/guardians", RequirePermission(service, "guardians:read"), peopleHandler.relationships)
	studentRoutes.PUT("/:id/guardians", RequirePermission(service, "guardians:manage"), RequireAdministrator(), peopleHandler.link)
	studentRoutes.DELETE("/:id/guardians/:guardianId", RequirePermission(service, "guardians:manage"), RequireAdministrator(), peopleHandler.unlink)
	guardianRoutes := router.Group("/guardians")
	guardianRoutes.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service), RequireModule(schoolService, "students"))
	guardianRoutes.GET("", RequirePermission(service, "guardians:read"), peopleHandler.guardians)
	guardianRoutes.POST("", RequirePermission(service, "guardians:manage"), RequireAdministrator(), peopleHandler.saveGuardian)
	guardianRoutes.GET("/me", RequirePermission(service, "guardians:read_children"), peopleHandler.myGuardian)
	guardianRoutes.GET("/me/children", RequirePermission(service, "guardians:read_children"), peopleHandler.children)
	guardianRoutes.GET("/me/children/:id", RequirePermission(service, "guardians:read_children"), peopleHandler.child)
	guardianRoutes.GET("/:id", RequirePermission(service, "guardians:read"), peopleHandler.guardian)
	guardianRoutes.GET("/:id/students", RequirePermission(service, "guardians:read"), peopleHandler.guardianStudents)
	guardianRoutes.PATCH("/:id", RequirePermission(service, "guardians:manage"), RequireAdministrator(), peopleHandler.saveGuardian)
	guardianRoutes.DELETE("/:id", RequirePermission(service, "guardians:manage"), RequireAdministrator(), peopleHandler.deleteGuardian)
	router.NoRoute(func(c *gin.Context) {
		failure(c, http.StatusNotFound, "not_found", "route not found")
	})
	router.NoMethod(func(c *gin.Context) {
		failure(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return router
}
