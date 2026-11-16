package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

// resetClientIP is the address the per-client rate limit counts against.
//
// The backend is not published: requests reach it through the web container,
// which rewrites X-Forwarded-For to the single address nginx itself saw (and
// nginx trusts that header only on the TLS edge's socket). So one value in
// the header is the client; anything else, including a direct call during
// development, falls back to the peer address. A forged header can only move
// an attempt into another bucket, never past the per-address limit, which is
// counted on the email address instead.
func resetClientIP(c *gin.Context) string {
	forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-For"))
	if forwarded != "" && !strings.Contains(forwarded, ",") {
		if ip := net.ParseIP(forwarded); ip != nil {
			return ip.String()
		}
	}
	return c.ClientIP()
}

// resetLanguage picks the language of the email. The form sends the language
// the person is reading the site in; anything else gets English.
func resetLanguage(raw string) string {
	if raw == "bn" {
		return "bn"
	}
	return "en"
}

type forgotPasswordRequest struct {
	Email    string `json:"email"`
	Language string `json:"language"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// forgotPasswordAvailable godoc
// @Summary Report whether the sign-in page can offer "forgot password"
// @Description True only when a mail relay is configured (SMTP_HOST). When false the page keeps telling people to ask the school office.
// @Tags authentication
// @Produce json
// @Router /auth/password/forgot [get]
func (h *authHandler) forgotPasswordAvailable(c *gin.Context) {
	success(c, http.StatusOK, gin.H{"enabled": h.service.PasswordResetEmailEnabled()})
}

// forgotPassword godoc
// @Summary Email a password reset link
// @Description Answers the same way whether or not the address has an account, so it cannot be used to find out who has one. The link works once and expires. Too many requests for one address answer 429.
// @Tags authentication
// @Accept json
// @Produce json
// @Router /auth/password/forgot [post]
func (h *authHandler) forgotPassword(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var request forgotPasswordRequest
	if !decodeSchoolJSON(c, 4*1024, &request) {
		return
	}
	if len(request.Email) > 254 {
		// Too long to be an address; answer as for any other address that
		// has no account.
		success(c, http.StatusAccepted, gin.H{"sent": true})
		return
	}
	err := h.service.RequestPasswordReset(c.Request.Context(), request.Email, resetClientIP(c), resetLanguage(request.Language))
	switch {
	case err == nil:
		// Deliberately the same answer for a known and an unknown address.
		success(c, http.StatusAccepted, gin.H{"sent": true})
	case errors.Is(err, auth.ErrResetRateLimited):
		// Safe to say: requests for addresses with no account are counted
		// too, so this reveals nothing about which addresses exist.
		failure(c, http.StatusTooManyRequests, "reset_rate_limited", "too many reset requests; try again in an hour or ask the school office")
	case errors.Is(err, auth.ErrResetUnavailable):
		failure(c, http.StatusServiceUnavailable, "reset_unavailable", "this school does not send password reset email; ask the school office")
	default:
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// resetPasswordTarget godoc
// @Summary Check an emailed reset link before showing the form
// @Tags authentication
// @Produce json
// @Router /auth/password/reset [get]
func (h *authHandler) resetPasswordTarget(c *gin.Context) {
	if !h.available(c) {
		return
	}
	user, err := h.service.PasswordResetTarget(c.Request.Context(), c.Query("token"))
	switch {
	case err == nil:
		success(c, http.StatusOK, gin.H{"valid": true, "display_name": user.DisplayName})
	case errors.Is(err, auth.ErrResetTokenInvalid):
		failure(c, http.StatusBadRequest, "invalid_reset_token", "this link is no longer valid; ask for a new one")
	case errors.Is(err, auth.ErrResetUnavailable):
		failure(c, http.StatusServiceUnavailable, "reset_unavailable", "this school does not send password reset email; ask the school office")
	default:
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// resetPassword godoc
// @Summary Set a new password with an emailed reset link
// @Description The link works once. Setting the password clears any lockout and signs every session out.
// @Tags authentication
// @Accept json
// @Produce json
// @Router /auth/password/reset [post]
func (h *authHandler) resetPassword(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var request resetPasswordRequest
	if !decodeSchoolJSON(c, 4*1024, &request) {
		return
	}
	_, revoked, err := h.service.CompletePasswordReset(c.Request.Context(), request.Token, request.NewPassword)
	switch {
	case err == nil:
		success(c, http.StatusOK, gin.H{"sessions_revoked": revoked})
	case errors.Is(err, auth.ErrWeakPassword):
		failure(c, http.StatusBadRequest, "weak_password", "new password must be 12 to 72 UTF-8 bytes")
	case errors.Is(err, auth.ErrResetTokenInvalid):
		failure(c, http.StatusBadRequest, "invalid_reset_token", "this link is no longer valid; ask for a new one")
	case errors.Is(err, auth.ErrResetUnavailable):
		failure(c, http.StatusServiceUnavailable, "reset_unavailable", "this school does not send password reset email; ask the school office")
	default:
		failure(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
