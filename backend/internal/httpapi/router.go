package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.CustomRecovery(func(c *gin.Context, _ any) {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		c.Abort()
	}))
	router.HandleMethodNotAllowed = true

	router.GET("/healthz", healthz)
	router.NoRoute(func(c *gin.Context) {
		failure(c, http.StatusNotFound, "not_found", "route not found")
	})
	router.NoMethod(func(c *gin.Context) {
		failure(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return router
}
