package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// securityHeaders sets conservative defaults on every API response. Handlers
// that serve files replace Content-Security-Policy with their own sandbox.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		c.Next()
	}
}

var (
	readinessMu sync.RWMutex
	readiness   func(context.Context) error
)

// SetReadiness registers the check behind /readyz, normally a database ping.
func SetReadiness(check func(context.Context) error) {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readiness = check
}

// readyz godoc
// @Summary Check readiness to serve requests
// @Description Returns 200 when the database answers within two seconds, otherwise 503. Error details are logged, never returned.
// @Tags health
// @Router /readyz [get]
func readyz(c *gin.Context) {
	readinessMu.RLock()
	check := readiness
	readinessMu.RUnlock()
	c.Header("Cache-Control", "no-store")
	if check == nil {
		failure(c, http.StatusServiceUnavailable, "not_ready", "database is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := check(ctx); err != nil {
		_ = c.Error(err)
		failure(c, http.StatusServiceUnavailable, "not_ready", "database is not reachable")
		return
	}
	success(c, http.StatusOK, HealthData{Status: "ready"})
}
