package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

func NewRouter() *gin.Engine {
	return NewRouterWithAuth(nil, true)
}

func NewRouterWithAuth(service *auth.Service, secureCookies bool) *gin.Engine {
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
	authRoutes.GET("/me", RequireAuth(service), RequirePermission("auth:read_self"), handler.me)
	router.NoRoute(func(c *gin.Context) {
		failure(c, http.StatusNotFound, "not_found", "route not found")
	})
	router.NoMethod(func(c *gin.Context) {
		failure(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return router
}
