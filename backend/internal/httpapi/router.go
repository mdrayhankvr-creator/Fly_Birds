package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
)

func NewRouter() *gin.Engine {
	return NewRouterWithAuth(nil, true)
}

func NewRouterWithAuth(service *auth.Service, secureCookies bool, authorization ...*rbac.Service) *gin.Engine {
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.Logger(), gin.CustomRecovery(func(c *gin.Context, _ any) {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		c.Abort()
	}))
	router.HandleMethodNotAllowed = true

	router.GET("/healthz", healthz)
	handler := &authHandler{service: service, secureCookies: secureCookies}
	authRoutes := router.Group("/auth")
	authRoutes.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") })
	authRoutes.POST("/login", handler.login)
	authRoutes.POST("/refresh", handler.refresh)
	authRoutes.POST("/logout", handler.logout)
	authRoutes.GET("/me", RequireAuth(service), RequirePermission(service, "auth:read_self"), handler.me)
	authRoutes.GET("/permissions", RequireAuth(service), RequirePermission(service, "auth:read_self"), handler.permissions)
	var grants *rbac.Service
	if len(authorization) > 0 {
		grants = authorization[0]
	}
	rbacHandler := &rbacHandler{service: grants}
	management := router.Group("")
	management.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") }, RequireAuth(service))
	management.GET("/roles", RequirePermission(service, "roles:read"), rbacHandler.roles)
	management.GET("/permissions", RequirePermission(service, "permissions:read"), rbacHandler.permissions)
	management.GET("/roles/:id/permissions", RequirePermission(service, "permissions:read"), rbacHandler.rolePermissions)
	management.PUT("/roles/:id/permissions", RequirePermission(service, "permissions:manage"), RequireAdministrator(), rbacHandler.setRolePermissions)
	management.GET("/users/:id/roles", RequirePermission(service, "roles:read"), rbacHandler.userRoles)
	management.PUT("/users/:id/roles", RequirePermission(service, "permissions:manage"), RequireAdministrator(), rbacHandler.setUserRoles)
	router.NoRoute(func(c *gin.Context) {
		failure(c, http.StatusNotFound, "not_found", "route not found")
	})
	router.NoMethod(func(c *gin.Context) {
		failure(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return router
}
