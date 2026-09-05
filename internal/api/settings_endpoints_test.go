package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestConfigGetAndPut(t *testing.T) {
	r := newStub()
	r.files["clamd.conf"] = "LogVerbose no\nMaxThreads 12\nUser clamav\n"
	r.out["systemctl restart clamav-daemon"] = ""
	s, cookies, csrf := authedServer(t, r)

	// GET only exposes whitelisted keys.
	gr := do(t, s, http.MethodGet, "/api/config/clamd", "", cookies, "")
	if gr.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", gr.Code, gr.Body)
	}
	if strings.Contains(gr.Body.String(), `"name":"User"`) {
		t.Error("non-whitelisted key exposed")
	}

	// PUT a whitelisted change with restart.
	pr := do(t, s, http.MethodPut, "/api/config/clamd",
		`{"updates":{"MaxThreads":["4"],"LogVerbose":["yes"]},"restart":true}`, cookies, csrf)
	if pr.Code != http.StatusOK {
		t.Fatalf("put: %d body=%s", pr.Code, pr.Body)
	}
	var resp struct {
		Restarted bool `json:"restarted"`
	}
	json.Unmarshal(pr.Body.Bytes(), &resp)
	if !resp.Restarted {
		t.Error("expected restarted=true")
	}
	if !strings.Contains(r.files["clamd.conf"], "MaxThreads 4") || !strings.Contains(r.files["clamd.conf"], "User clamav") {
		t.Errorf("conf not written correctly:\n%s", r.files["clamd.conf"])
	}

	// PUT a non-whitelisted key is rejected.
	pr = do(t, s, http.MethodPut, "/api/config/clamd", `{"updates":{"User":["root"]}}`, cookies, csrf)
	if pr.Code != http.StatusBadRequest {
		t.Errorf("non-whitelisted put: status = %d", pr.Code)
	}
}

func TestOnAccessGetPut(t *testing.T) {
	r := newStub()
	r.have["clamonacc"] = true
	r.files["clamd.conf"] = "LogVerbose no\n"
	r.out["systemctl show clamav-daemon"+showProps] = "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"
	r.out["systemctl show clamav-clamonacc"+showProps] = "Id=clamav-clamonacc.service\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n"
	r.out["systemctl restart clamav-daemon"] = ""
	r.out["systemctl enable clamav-clamonacc"] = ""
	r.out["systemctl restart clamav-clamonacc"] = ""
	r.out["systemctl show clamav-daemon"+showProps] = "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"
	r.out["systemctl enable clamav-daemon"] = ""
	s, cookies, csrf := authedServer(t, r)
	s.cfg.BrowseRoot = "/"

	gr := do(t, s, http.MethodGet, "/api/onaccess", "", cookies, "")
	if gr.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", gr.Code, gr.Body)
	}

	body := `{"enabled":true,"paths":["/srv/watched"],"prevention":true}`
	pr := do(t, s, http.MethodPut, "/api/onaccess", body, cookies, csrf)
	if pr.Code != http.StatusOK {
		t.Fatalf("put: %d body=%s", pr.Code, pr.Body)
	}
	if !strings.Contains(r.files["clamd.conf"], "OnAccessIncludePath") ||
		!strings.Contains(r.files["clamd.conf"], "OnAccessPrevention yes") {
		t.Errorf("clamd.conf not updated:\n%s", r.files["clamd.conf"])
	}
}

func TestChangePassword(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())

	// Wrong current password.
	rec := do(t, s, http.MethodPost, "/api/password",
		`{"current":"nope","new":"another-good-one"}`, cookies, csrf)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current: status = %d", rec.Code)
	}

	// Too-short new password.
	rec = do(t, s, http.MethodPost, "/api/password",
		`{"current":"a-good-password","new":"short"}`, cookies, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short new: status = %d", rec.Code)
	}

	// Success invalidates the session.
	rec = do(t, s, http.MethodPost, "/api/password",
		`{"current":"a-good-password","new":"a-new-good-password"}`, cookies, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change: status = %d body=%s", rec.Code, rec.Body)
	}
	if s.sessions.Count() != 0 {
		t.Errorf("sessions not cleared: %d", s.sessions.Count())
	}
	// The old cookie no longer authenticates.
	rec = do(t, s, http.MethodGet, "/api/activity", "", cookies, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("stale session still works: %d", rec.Code)
	}
}

func TestNotificationsConfigRoundTrip(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())

	put := `{"enabled":true,"events":["scan-detection"],"webhook":{"url":"https://example.test/hook"},"smtp":{"host":"mail","port":587,"password":"topsecret","from":"a@b","to":"c@d"}}`
	pr := do(t, s, http.MethodPut, "/api/notifications", put, cookies, csrf)
	if pr.Code != http.StatusOK {
		t.Fatalf("put: %d body=%s", pr.Code, pr.Body)
	}
	if strings.Contains(pr.Body.String(), "topsecret") {
		t.Error("GET/PUT response leaked the SMTP password")
	}

	gr := do(t, s, http.MethodGet, "/api/notifications", "", cookies, "")
	var cfg struct {
		Enabled bool     `json:"enabled"`
		Events  []string `json:"events"`
	}
	json.Unmarshal(gr.Body.Bytes(), &cfg)
	if !cfg.Enabled || len(cfg.Events) != 1 {
		t.Fatalf("config not persisted: %+v", cfg)
	}

	// Test with an unreachable webhook should surface an error.
	tr := do(t, s, http.MethodPost, "/api/notifications/test", "", cookies, csrf)
	if tr.Code != http.StatusBadGateway {
		t.Errorf("test send: status = %d, want 502", tr.Code)
	}
}
