package httpapi

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// PostgreSQL text columns cannot store NUL, and valid UTF-8 may contain it.
// Reject it at the boundary so requests fail with 400 instead of a database error.
func rejectNULQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.IndexByte(c.Request.URL.Path, 0) >= 0 {
			failure(c, http.StatusBadRequest, "invalid_request", "NUL characters are not allowed")
			c.Abort()
			return
		}
		for key, values := range c.Request.URL.Query() {
			if strings.IndexByte(key, 0) >= 0 {
				failure(c, http.StatusBadRequest, "invalid_request", "NUL characters are not allowed")
				c.Abort()
				return
			}
			for _, value := range values {
				if strings.IndexByte(value, 0) >= 0 {
					failure(c, http.StatusBadRequest, "invalid_request", "NUL characters are not allowed")
					c.Abort()
					return
				}
			}
		}
		c.Next()
	}
}

// containsJSONNUL reports a raw NUL byte or a \u0000 escape. An escaped
// backslash followed by the text "u0000" is ordinary text and is allowed.
func containsJSONNUL(body []byte) bool {
	if bytes.IndexByte(body, 0) >= 0 {
		return true
	}
	for i := 0; i+5 < len(body); i++ {
		if body[i] != '\\' {
			continue
		}
		if body[i+1] == '\\' {
			i++
			continue
		}
		if body[i+1] == 'u' && string(body[i+2:i+6]) == "0000" {
			return true
		}
	}
	return false
}
