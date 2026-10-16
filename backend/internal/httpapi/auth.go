package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

const (
	refreshCookie = "flybirds_refresh"
	csrfCookie    = "flybirds_csrf"
)

type authHandler struct {
	service       *auth.Service
	secureCookies bool
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func decodeLogin(c *gin.Context) (loginRequest, bool) {
	var request loginRequest
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		failure(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "application/json is required")
		return loginRequest{}, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		failure(c, http.StatusBadRequest, "invalid_request", "valid email and password are required")
		return loginRequest{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		failure(c, http.StatusBadRequest, "invalid_request", "valid email and password are required")
		return loginRequest{}, false
	}
	if _, err := auth.NormalizeEmail(request.Email); err != nil || request.Password == "" || len(request.Password) > 72 {
		failure(c, http.StatusBadRequest, "invalid_request", "valid email and password are required")
		return loginRequest{}, false
	}
	return request, true
}

func (h *authHandler) available(c *gin.Context) bool {
	if h.service != nil {
		return true
	}
	failure(c, http.StatusServiceUnavailable, "auth_unavailable", "authentication requires a configured database")
	return false
}

func (h *authHandler) setCookies(c *gin.Context, credentials auth.Credentials) {
	expires := time.Now().Add(auth.RefreshLifetime)
	for _, cookie := range []*http.Cookie{
		{Name: refreshCookie, Value: credentials.RefreshToken, HttpOnly: true},
		{Name: csrfCookie, Value: credentials.CSRFToken, HttpOnly: false},
	} {
		cookie.Path = "/"
		cookie.Secure = h.secureCookies
		cookie.SameSite = http.SameSiteStrictMode
		cookie.Expires = expires
		cookie.MaxAge = int(auth.RefreshLifetime.Seconds())
		http.SetCookie(c.Writer, cookie)
	}
}

func (h *authHandler) clearCookies(c *gin.Context) {
	for _, name := range []string{refreshCookie, csrfCookie} {
		http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/", Secure: h.secureCookies,
			HttpOnly: name == refreshCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
}

func csrfCookies(c *gin.Context) (string, string, bool) {
	refresh, refreshErr := c.Cookie(refreshCookie)
	csrf, csrfErr := c.Cookie(csrfCookie)
	header := c.GetHeader("X-CSRF-Token")
	if refreshErr != nil || csrfErr != nil || csrf == "" || header == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(header)) != 1 {
		return "", "", false
	}
	return refresh, csrf, true
}

// login godoc
// @Summary Log in with email and password
// @Tags authentication
// @Accept json
// @Produce json
// @Router /auth/login [post]
func (h *authHandler) login(c *gin.Context) {
	if !h.available(c) {
		return
	}
	request, valid := decodeLogin(c)
	if !valid {
		return
	}
	credentials, err := h.service.Login(c.Request.Context(), request.Email, request.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		failure(c, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if err != nil {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.setCookies(c, credentials)
	success(c, http.StatusOK, credentials)
}

// refresh godoc
// @Summary Rotate refresh token and issue a new access token
// @Tags authentication
// @Produce json
// @Router /auth/refresh [post]
func (h *authHandler) refresh(c *gin.Context) {
	if !h.available(c) {
		return
	}
	refresh, csrf, ok := csrfCookies(c)
	if !ok {
		failure(c, http.StatusUnauthorized, "invalid_session", "session is invalid or expired")
		return
	}
	credentials, err := h.service.Refresh(c.Request.Context(), refresh, csrf)
	if errors.Is(err, auth.ErrInvalidSession) {
		h.clearCookies(c)
		failure(c, http.StatusUnauthorized, "invalid_session", "session is invalid or expired")
		return
	}
	if err != nil {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.setCookies(c, credentials)
	success(c, http.StatusOK, credentials)
}

// logout godoc
// @Summary Revoke the current refresh session
// @Tags authentication
// @Router /auth/logout [post]
func (h *authHandler) logout(c *gin.Context) {
	if !h.available(c) {
		return
	}
	refresh, csrf, ok := csrfCookies(c)
	if !ok {
		failure(c, http.StatusUnauthorized, "invalid_session", "session is invalid or expired")
		return
	}
	if err := h.service.Logout(c.Request.Context(), refresh, csrf); err != nil && !errors.Is(err, auth.ErrInvalidSession) {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.clearCookies(c)
	c.Status(http.StatusNoContent)
}

// me godoc
// @Summary Get the current signed-in user
// @Tags authentication
// @Produce json
// @Router /auth/me [get]
func (h *authHandler) me(c *gin.Context) {
	user, _ := c.Get("auth_user")
	success(c, http.StatusOK, user)
}

// permissions godoc
// @Summary Get current server-side grants for the signed-in user
// @Tags authentication
// @Router /auth/permissions [get]
func (h *authHandler) permissions(c *gin.Context) {
	user := c.MustGet("auth_user").(auth.User)
	ids, err := h.service.PermissionsForUser(c.Request.Context(), user.ID)
	if err != nil {
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	success(c, http.StatusOK, ids)
}

func RequireAuth(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			failure(c, http.StatusServiceUnavailable, "auth_unavailable", "authentication requires a configured database")
			c.Abort()
			return
		}
		authorization := c.GetHeader("Authorization")
		bearer, token, ok := strings.Cut(authorization, " ")
		if !ok || !strings.EqualFold(bearer, "Bearer") || token == "" || strings.Contains(token, " ") {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		user, err := service.Authenticate(c.Request.Context(), token)
		if errors.Is(err, auth.ErrInvalidToken) {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		if err != nil {
			failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
			c.Abort()
			return
		}
		c.Set("auth_user", user)
		c.Next()
	}
}

// RequirePermission reads the current role grant on every request. RequireAuth
// must precede it in the route chain; it never trusts token role claims.
func RequirePermission(service *auth.Service, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, exists := c.Get("auth_user")
		if !exists || service == nil {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		user, valid := value.(auth.User)
		if !valid {
			failure(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		allowed, err := service.HasPermission(c.Request.Context(), user.ID, permission)
		if err != nil {
			failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
			c.Abort()
			return
		}
		if !allowed {
			failure(c, http.StatusForbidden, "forbidden", "permission denied")
			c.Abort()
			return
		}
		c.Next()
	}
}
