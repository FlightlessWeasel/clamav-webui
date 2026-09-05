package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

// stubRunner implements clamav.Runner and clamav.FS with canned answers keyed
// by the joined command line / file basename.
type stubRunner struct {
	out   map[string]string
	err   map[string]error
	exit  map[string]int
	have  map[string]bool
	lines map[string][]string
	files map[string]string // basename -> content ("" ok); presence = Stat succeeds
}

func newStub() *stubRunner {
	return &stubRunner{
		out: map[string]string{}, err: map[string]error{}, exit: map[string]int{},
		have: map[string]bool{}, lines: map[string][]string{}, files: map[string]string{},
	}
}

func ckey(c clamav.Cmd) string { return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " ")) }

func (s *stubRunner) Stat(name string) (os.FileInfo, error) {
	if _, ok := s.files[path.Base(name)]; ok {
		return stubInfo{name: path.Base(name)}, nil
	}
	return nil, os.ErrNotExist
}

func (s *stubRunner) ReadFile(name string) ([]byte, error) {
	if c, ok := s.files[path.Base(name)]; ok {
		return []byte(c), nil
	}
	return nil, os.ErrNotExist
}

func (s *stubRunner) WriteFile(name string, data []byte, _ os.FileMode) error {
	s.files[path.Base(name)] = string(data)
	return nil
}

func (s *stubRunner) Rename(oldPath, newPath string) error {
	ob, nb := path.Base(oldPath), path.Base(newPath)
	c, ok := s.files[ob]
	if !ok {
		return os.ErrNotExist
	}
	s.files[nb] = c
	delete(s.files, ob)
	return nil
}

func (s *stubRunner) Remove(name string) error {
	delete(s.files, path.Base(name))
	return nil
}

type stubInfo struct{ name string }

func (i stubInfo) Name() string       { return i.name }
func (i stubInfo) Size() int64        { return 1024 }
func (i stubInfo) Mode() fs.FileMode  { return 0o644 }
func (i stubInfo) ModTime() time.Time { return time.Now().Add(-time.Hour) }
func (i stubInfo) IsDir() bool        { return false }
func (i stubInfo) Sys() any           { return nil }

func (s *stubRunner) Run(_ context.Context, c clamav.Cmd) (clamav.Result, error) {
	k := ckey(c)
	return clamav.Result{Stdout: []byte(s.out[k])}, s.err[k]
}
func (s *stubRunner) Stream(_ context.Context, c clamav.Cmd, onLine func(string)) error {
	k := ckey(c)
	for _, l := range s.lines[k] {
		onLine(l)
	}
	if s.out[k] != "" {
		for _, l := range strings.Split(strings.TrimRight(s.out[k], "\n"), "\n") {
			onLine(l)
		}
	}
	if n := s.exit[k]; n != 0 {
		return &clamav.ExitError{Code: n}
	}
	return s.err[k]
}
func (s *stubRunner) LookPath(name string) (string, bool) {
	if s.have[name] {
		return "/usr/bin/" + name, true
	}
	return "", false
}

const showProps = " --property=Id,LoadState,ActiveState,SubState,UnitFileState,ActiveEnterTimestamp"

// batchShowKey/blocks mirror how clamav.Services queries all managed units in
// one `systemctl show` call.
func batchShowKey() string {
	return "systemctl show " + strings.Join(clamav.ManagedUnits, " ") + showProps
}

func showBlocks(active string) string {
	sub := "dead"
	if active == "active" {
		sub = "running"
	}
	var b strings.Builder
	for i, u := range clamav.ManagedUnits {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Id=" + u + ".service\nLoadState=loaded\nActiveState=" + active +
			"\nSubState=" + sub + "\nUnitFileState=enabled\n")
	}
	return b.String()
}

// authedServer returns a server with setup done, a session cookie, the CSRF
// token, and its clam Manager pointed at the given runner.
func authedServer(t *testing.T, r clamav.Runner) (*Server, []*http.Cookie, string) {
	t.Helper()
	s := newTestServer(t)
	if fsys, ok := r.(clamav.FS); ok {
		s.clam = clamav.NewManagerWithDeps(config.Defaults(), r, fsys)
	} else {
		s.clam = clamav.NewManagerWithRunner(config.Defaults(), r)
	}

	rec := do(t, s, http.MethodPost, "/api/setup", `{"password":"a-good-password"}`, nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("setup: %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	return s, cookies, cookieNamed(cookies, csrfCookie).Value
}

func TestServicesEndpoint(t *testing.T) {
	r := newStub()
	r.out[batchShowKey()] = showBlocks("inactive")
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
	r.out["systemctl show clamav-daemon"+showProps] = "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"
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

func TestSignaturesEndpoint(t *testing.T) {
	r := newStub()
	r.have["freshclam"] = true
	r.have["sigtool"] = true
	r.files["daily.cld"] = ""
	r.files["main.cvd"] = ""
	r.files["freshclam.conf"] = "Checks 24\n"
	r.out["sigtool --info /var/lib/clamav/daily.cld"] = "Version: 27000\nSignatures: 2000000\n"
	r.out["sigtool --info /var/lib/clamav/main.cvd"] = "Version: 62\nSignatures: 6000000\n"
	r.out["systemctl show clamav-freshclam"+showProps] = "Id=clamav-freshclam.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"
	s, cookies, _ := authedServer(t, r)

	rec := do(t, s, http.MethodGet, "/api/signatures", "", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var sig struct {
		TotalSigs int `json:"total_sigs"`
		Checks    int `json:"checks"`
		Databases []struct {
			Name    string `json:"name"`
			Present bool   `json:"present"`
		} `json:"databases"`
		FreshclamService clamav.ServiceState `json:"freshclam_service"`
	}
	json.Unmarshal(rec.Body.Bytes(), &sig)
	if sig.TotalSigs != 8000000 || sig.Checks != 24 {
		t.Errorf("sig = %+v", sig)
	}
	if len(sig.Databases) != 3 {
		t.Fatalf("want 3 db rows, got %d", len(sig.Databases))
	}
	if sig.FreshclamService.Active != "active" {
		t.Errorf("freshclam svc = %+v", sig.FreshclamService)
	}
}

func TestSignaturesUpdateEnqueuesJob(t *testing.T) {
	r := newStub()
	r.have["freshclam"] = true
	r.out["systemctl show clamav-freshclam"+showProps] = "Id=clamav-freshclam.service\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n"
	r.lines["freshclam --stdout"] = []string{"daily.cld updated (version: 27001)"}
	s, cookies, csrf := authedServer(t, r)

	rec := do(t, s, http.MethodPost, "/api/signatures/update", "", cookies, csrf)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		JobID int64 `json:"job_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.JobID == 0 {
		t.Fatal("no job id")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		jr := do(t, s, http.MethodGet, "/api/jobs/"+strconv.FormatInt(body.JobID, 10), "", cookies, "")
		var job struct {
			Status string `json:"status"`
			Log    string `json:"log"`
		}
		json.Unmarshal(jr.Body.Bytes(), &job)
		if job.Status == "done" {
			if !strings.Contains(job.Log, "daily.cld updated") {
				t.Errorf("job log = %q", job.Log)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("freshclam job did not finish")
}

func TestDashboardEndpoint(t *testing.T) {
	r := newStub()
	r.have["clamscan"] = true
	r.out["clamscan --version"] = "ClamAV 1.0.3/27000/Fri Sep 20 08:15:11 2024"
	r.out["apt-cache policy clamav"] = "clamav:\n  Installed: 1.0.3+dfsg-1\n  Candidate: 1.0.3+dfsg-1\n"
	r.out[batchShowKey()] = showBlocks("active")
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
