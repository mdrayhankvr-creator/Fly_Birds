package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
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

func NewRouterWithAll(service *auth.Service, secureCookies bool, grants *rbac.Service, schoolService *school.Service, userService *users.Service, auditService *audit.Service, dashboardService *dashboard.Service) *gin.Engine {
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
	router.NoRoute(func(c *gin.Context) {
		failure(c, http.StatusNotFound, "not_found", "route not found")
	})
	router.NoMethod(func(c *gin.Context) {
		failure(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return router
}
