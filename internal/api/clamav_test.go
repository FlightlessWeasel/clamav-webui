package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

// stubRunner implements clamav.Runner with canned answers keyed by the joined
// command line.
type stubRunner struct {
	out   map[string]string
	err   map[string]error
	have  map[string]bool
	lines map[string][]string
}

func newStub() *stubRunner {
	return &stubRunner{out: map[string]string{}, err: map[string]error{}, have: map[string]bool{}, lines: map[string][]string{}}
}

func ckey(c clamav.Cmd) string { return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " ")) }

func (s *stubRunner) Run(_ context.Context, c clamav.Cmd) (clamav.Result, error) {
	k := ckey(c)
	return clamav.Result{Stdout: []byte(s.out[k])}, s.err[k]
}
func (s *stubRunner) Stream(_ context.Context, c clamav.Cmd, onLine func(string)) error {
	k := ckey(c)
	for _, l := range s.lines[k] {
		onLine(l)
	}
	return s.err[k]
}
func (s *stubRunner) LookPath(name string) (string, bool) {
	if s.have[name] {
		return "/usr/bin/" + name, true
	}
	return "", false
}

const showProps = " --property=LoadState,ActiveState,SubState,UnitFileState,ActiveEnterTimestampMonotonic,ActiveEnterTimestamp"

// authedServer returns a server with setup done, a session cookie, the CSRF
// token, and its clam Manager pointed at the given runner.
func authedServer(t *testing.T, r clamav.Runner) (*Server, []*http.Cookie, string) {
	t.Helper()
	s := newTestServer(t)
	s.clam = clamav.NewManagerWithRunner(config.Defaults(), r)

	rec := do(t, s, http.MethodPost, "/api/setup", `{"password":"a-good-password"}`, nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("setup: %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	return s, cookies, cookieNamed(cookies, csrfCookie).Value
}

func TestServicesEndpoint(t *testing.T) {
	r := newStub()
	for _, u := range clamav.ManagedUnits {
		r.out["systemctl show "+u+showProps] = "LoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n"
	}
	s, cookies, _ := authedServer(t, r)

	rec := do(t, s, http.MethodGet, "/api/services", "", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Services []clamav.ServiceState `json:"services"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Services) != len(clamav.ManagedUnits) {
		t.Fatalf("got %d services", len(body.Services))
	}
}

func TestServiceActionEndpointValidates(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())

	rec := do(t, s, http.MethodPost, "/api/services/sshd/stop", "", cookies, csrf)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unmanaged unit: status = %d", rec.Code)
	}
	rec = do(t, s, http.MethodPost, "/api/services/clamav-daemon/sabotage", "", cookies, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad action: status = %d", rec.Code)
	}
}

func TestServiceActionEndpointRuns(t *testing.T) {
	r := newStub()
	r.out["systemctl restart clamav-daemon"] = ""
	r.out["systemctl show clamav-daemon"+showProps] = "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"
	s, cookies, csrf := authedServer(t, r)

	rec := do(t, s, http.MethodPost, "/api/services/clamav-daemon/restart", "", cookies, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var st clamav.ServiceState
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Active != "active" {
		t.Errorf("returned state = %+v", st)
	}
}

func TestInstallEndpointEnqueuesJob(t *testing.T) {
	r := newStub()
	r.lines["apt-get update"] = []string{"Reading package lists..."}
	r.lines["apt-get install -y clamav clamav-daemon clamav-freshclam"] = []string{"Setting up clamav ..."}
	s, cookies, csrf := authedServer(t, r)

	rec := do(t, s, http.MethodPost, "/api/clamav/install", "", cookies, csrf)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		JobID int64 `json:"job_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.JobID == 0 {
		t.Fatal("no job_id returned")
	}

	// Poll the job endpoint until it completes.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		jr := do(t, s, http.MethodGet, "/api/jobs/"+strconv.FormatInt(body.JobID, 10), "", cookies, "")
		var job struct {
			Status string `json:"status"`
			Log    string `json:"log"`
		}
		json.Unmarshal(jr.Body.Bytes(), &job)
		if job.Status == "done" {
			if !strings.Contains(job.Log, "Setting up clamav") {
				t.Errorf("job log missing apt output: %q", job.Log)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("install job did not finish")
}

func TestDashboardEndpoint(t *testing.T) {
	r := newStub()
	r.have["clamscan"] = true
	r.out["clamscan --version"] = "ClamAV 1.0.3/27000/Fri Sep 20 08:15:11 2024"
	r.out["apt-cache policy clamav"] = "clamav:\n  Installed: 1.0.3+dfsg-1\n  Candidate: 1.0.3+dfsg-1\n"
	for _, u := range clamav.ManagedUnits {
		r.out["systemctl show "+u+showProps] = "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"
	}
	s, cookies, _ := authedServer(t, r)

	rec := do(t, s, http.MethodGet, "/api/dashboard", "", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Install struct {
			Installed     bool   `json:"installed"`
			EngineVersion string `json:"engine_version"`
		} `json:"install"`
		Services []clamav.ServiceState `json:"services"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Install.Installed || body.Install.EngineVersion != "1.0.3" {
		t.Errorf("install = %+v", body.Install)
	}
	if len(body.Services) != 3 {
		t.Errorf("services len = %d", len(body.Services))
	}
}
