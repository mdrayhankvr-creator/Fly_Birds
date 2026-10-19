package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthz(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Status != "ok" {
		t.Fatalf("health status = %q, want ok", body.Data.Status)
	}
}

func TestUnknownRouteUsesStandardError(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want not_found", body.Error.Code)
	}
}

func TestPanicUsesStandardError(t *testing.T) {
	router := NewRouter()
	router.GET("/panic", func(c *gin.Context) { panic("test panic") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "internal_error" {
		t.Fatalf("error code = %q, want internal_error", body.Error.Code)
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	for _, path := range []string{"/healthz", "/missing", "/auth/me"} {
		response := httptest.NewRecorder()
		NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		for header, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'"} {
			if got := response.Header().Get(header); got != want {
				t.Errorf("%s %s = %q, want %q", path, header, got, want)
			}
		}
	}
}

func TestReadyzReportsDatabaseState(t *testing.T) {
	t.Cleanup(func() { SetReadiness(nil) })
	check := func(want int, body string) {
		t.Helper()
		response := httptest.NewRecorder()
		NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if response.Code != want || !strings.Contains(response.Body.String(), body) {
			t.Fatalf("readyz = %d %s, want %d containing %q", response.Code, response.Body.String(), want, body)
		}
	}
	SetReadiness(nil)
	check(http.StatusServiceUnavailable, "not_ready")
	SetReadiness(func(context.Context) error { return errors.New("dial tcp 10.0.0.5:5432: secret detail") })
	check(http.StatusServiceUnavailable, "database is not reachable")
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if strings.Contains(response.Body.String(), "secret detail") {
		t.Fatal("readiness leaked the database error")
	}
	SetReadiness(func(context.Context) error { return nil })
	check(http.StatusOK, "ready")
}
