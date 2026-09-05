package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func do(t *testing.T, s *Server, method, path, body string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	if csrf != "" {
		r.Header.Set(csrfHeader, csrf)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	return rec
}

func cookieNamed(recs []*http.Cookie, name string) *http.Cookie {
	for _, c := range recs {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestSetupThenLoginFlow(t *testing.T) {
	s := newTestServer(t)

	// Status before setup.
	rec := do(t, s, http.MethodGet, "/api/status", "", nil, "")
	var st map[string]any
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st["setup_complete"] != false || st["authenticated"] != false {
		t.Fatalf("pre-setup status = %v", st)
	}

	// Too-short password rejected.
	rec = do(t, s, http.MethodPost, "/api/setup", `{"password":"short"}`, nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: status = %d", rec.Code)
	}

	// Setup succeeds and sets cookies.
	rec = do(t, s, http.MethodPost, "/api/setup", `{"password":"a-good-password"}`, nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("setup: status = %d body=%s", rec.Code, rec.Body)
	}
	setCookies := rec.Result().Cookies()
	if cookieNamed(setCookies, sessionCookie) == nil || cookieNamed(setCookies, csrfCookie) == nil {
		t.Fatalf("setup did not set session+csrf cookies: %v", setCookies)
	}

	// Second setup is refused.
	rec = do(t, s, http.MethodPost, "/api/setup", `{"password":"another-password"}`, nil, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup: status = %d", rec.Code)
	}

	// Authenticated status via the session cookie.
	rec = do(t, s, http.MethodGet, "/api/status", "", setCookies, "")
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st["setup_complete"] != true || st["authenticated"] != true {
		t.Fatalf("post-setup status = %v", st)
	}

	// Wrong password login.
	rec = do(t, s, http.MethodPost, "/api/login", `{"password":"nope"}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: status = %d", rec.Code)
	}

	// Correct password login.
	rec = do(t, s, http.MethodPost, "/api/login", `{"password":"a-good-password"}`, nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("good login: status = %d", rec.Code)
	}
	loginCookies := rec.Result().Cookies()

	// Logout without CSRF header is forbidden.
	rec = do(t, s, http.MethodPost, "/api/logout", "", loginCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf: status = %d", rec.Code)
	}

	// Logout with CSRF header succeeds.
	csrf := cookieNamed(loginCookies, csrfCookie).Value
	rec = do(t, s, http.MethodPost, "/api/logout", "", loginCookies, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: status = %d", rec.Code)
	}

	// Session is dead now.
	rec = do(t, s, http.MethodPost, "/api/logout", "", loginCookies, csrf)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("logout after logout: status = %d", rec.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	s := newTestServer(t)
	do(t, s, http.MethodPost, "/api/setup", `{"password":"a-good-password"}`, nil, "")

	var last int
	for i := 0; i < 8; i++ {
		rec := do(t, s, http.MethodPost, "/api/login", `{"password":"wrong"}`, nil, "")
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", last)
	}
}

func TestRequireAuthBlocksAnon(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodPost, "/api/logout", "", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon logout: status = %d", rec.Code)
	}
}
