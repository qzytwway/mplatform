package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/common"
)

func TestCredentialLoginLimiterBlocksLoopbackClientIP(t *testing.T) {
	limiter := &credentialLoginAttemptLimiter{
		attempts: map[string]credentialLoginAttemptState{},
	}

	clientIP := "127.0.0.1"
	for i := 0; i < credentialLoginMaxFailures; i++ {
		if limiter.recordFailure(clientIP) {
			t.Fatalf("recordFailure() blocked after %d failures, want not blocked yet", i+1)
		}
	}
	if !limiter.recordFailure(clientIP) {
		t.Fatalf("recordFailure() did not block loopback client IP")
	}
	if !limiter.isBlocked(clientIP) {
		t.Fatalf("isBlocked() = false, want true for loopback client IP")
	}
}

func TestSetCookieSecure(t *testing.T) {
	originalHost := common.Host
	t.Cleanup(func() {
		common.Host = originalHost
	})

	t.Run("secure when host is https", func(t *testing.T) {
		common.Host = "https://mplatform.example.com"
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "http://mplatform.local", nil)

		setCookieSecure(c, "auth_token", "value", 600)

		cookie := rec.Result().Cookies()[0]
		if !cookie.Secure {
			t.Fatalf("cookie.Secure = false, want true")
		}
		if !cookie.HttpOnly {
			t.Fatalf("cookie.HttpOnly = false, want true")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie.SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
		}
		if got := rec.Header().Get("Set-Cookie"); !strings.Contains(got, "SameSite=Lax") {
			t.Fatalf("Set-Cookie header = %q, want SameSite=Lax", got)
		}
	})

	t.Run("secure when forwarded proto is https", func(t *testing.T) {
		common.Host = ""
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		req := httptest.NewRequest(http.MethodGet, "http://mplatform.local", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		c.Request = req

		setCookieSecure(c, "auth_token", "value", 600)

		if !rec.Result().Cookies()[0].Secure {
			t.Fatalf("cookie.Secure = false, want true")
		}
	})

	t.Run("not secure on plain http", func(t *testing.T) {
		common.Host = ""
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "http://mplatform.local", nil)

		setCookieSecure(c, "auth_token", "value", 600)

		if rec.Result().Cookies()[0].Secure {
			t.Fatalf("cookie.Secure = true, want false")
		}
	})
}
