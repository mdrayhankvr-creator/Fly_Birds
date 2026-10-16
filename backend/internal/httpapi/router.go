package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academicsessions"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
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

func NewRouterWithAcademicServices(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, sectionServices ...*academic.SectionService) *gin.Engine {
	var sectionService *academic.SectionService
	if len(sectionServices) > 0 {
		sectionService = sectionServices[0]
	}
	return NewRouterWithPeople(service, secureCookies, grants, schoolService, userService, auditService, dashboardService, sessionService, academicService, sectionService, nil)
}

func NewRouterWithPeople(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service, sessionService *academicsessions.Service, academicService *academic.Service, sectionService *academic.SectionService, peopleService *people.Service) *gin.Engine {
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
