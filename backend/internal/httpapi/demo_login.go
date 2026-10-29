package httpapi

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type demoLoginRequest struct {
	Role string `json:"role"`
}

// localHostname reports whether host (optionally with a port) names this
// machine: localhost or a loopback address.
func localHostname(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// demoRequestIsLocal accepts only requests addressed to localhost/127.0.0.1
// (the Host header, which the web server passes through) and, for sign-in,
// sent by a page on such an origin. Browsers set Origin and Sec-Fetch-Site
// themselves, so another website cannot make a visitor's browser use this.
// A non-browser client can forge these headers; the real protection is that
// the feature is off unless DEMO_QUICK_LOGIN=true.
func demoRequestIsLocal(c *gin.Context, requireOrigin bool) bool {
	if !localHostname(c.Request.Host) {
		return false
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		return !requireOrigin
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !localHostname(parsed.Host) {
		return false
	}
	if site := c.GetHeader("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	return true
}

// demoLoginInfo tells the sign-in form whether to show the quick-login
// buttons. When the feature is off, or the page is not on localhost, the
// route does not exist as far as the browser can tell.
func (h *authHandler) demoLoginInfo(c *gin.Context) {
	if !h.service.DemoQuickLoginEnabled() || !demoRequestIsLocal(c, false) {
		failure(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	success(c, http.StatusOK, gin.H{"roles": auth.DemoQuickLoginRoles})
}

// demoLogin signs in as a seed-demo account without a password.
func (h *authHandler) demoLogin(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if !h.service.DemoQuickLoginEnabled() {
		failure(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	if !demoRequestIsLocal(c, true) {
		failure(c, http.StatusForbidden, "local_only", "demo quick login works only from localhost")
		return
	}
	var request demoLoginRequest
	if !decodeSchoolJSON(c, 256, &request) {
		return
	}
	credentials, err := h.service.DemoLogin(c.Request.Context(), request.Role, map[string]any{"host": c.Request.Host, "origin": c.GetHeader("Origin")})
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		failure(c, http.StatusForbidden, "not_demo_account", "only seed-demo accounts can use quick login")
		return
	case errors.Is(err, auth.ErrDemoUnavailable):
		failure(c, http.StatusNotFound, "not_found", "not found")
		return
	case err != nil:
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.setCookies(c, credentials)
	success(c, http.StatusOK, credentials)
}
