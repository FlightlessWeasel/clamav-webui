package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCreateScanRunsAndPersistsFindings(t *testing.T) {
	r := newStub()
	r.have["clamscan"] = true
	r.out["clamscan --version"] = "ClamAV 1.0.3/27000/Fri Sep 20 08:15:11 2024"
	r.out["clamscan --stdout --recursive /tmp/scan"] = strings.Join([]string{
		"/tmp/scan/clean.txt: OK",
		"/tmp/scan/eicar.com: Win.Test.EICAR_HDB-1 FOUND",
		"----------- SCAN SUMMARY -----------",
		"Engine version: 1.0.3",
		"Scanned files: 2",
		"Infected files: 1",
		"Time: 1.100 sec (0 m 1 s)",
	}, "\n")
	r.exit["clamscan --stdout --recursive /tmp/scan"] = 1
	s, cookies, csrf := authedServer(t, r)

	rec := do(t, s, http.MethodPost, "/api/scans",
		`{"paths":["/tmp/scan"],"options":{"recursive":true}}`, cookies, csrf)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var created struct {
		ScanID int64 `json:"scan_id"`
		JobID  int64 `json:"job_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ScanID == 0 || created.JobID == 0 {
		t.Fatalf("ids = %+v", created)
	}

	// Wait for the scan to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gr := do(t, s, http.MethodGet, "/api/scans/"+strconv.FormatInt(created.ScanID, 10), "", cookies, "")
		var scan struct {
			Status   string `json:"status"`
			Scanned  int    `json:"scanned"`
			Infected int    `json:"infected"`
			Engine   string `json:"engine"`
		}
		json.Unmarshal(gr.Body.Bytes(), &scan)
		if scan.Status == "done" {
			if scan.Scanned != 2 || scan.Infected != 1 || scan.Engine != "1.0.3" {
				t.Fatalf("scan row = %+v", scan)
			}
			fr := do(t, s, http.MethodGet, "/api/scans/"+strconv.FormatInt(created.ScanID, 10)+"/findings", "", cookies, "")
			var fb struct {
				Findings []struct {
					Path      string `json:"path"`
					Signature string `json:"signature"`
				} `json:"findings"`
			}
			json.Unmarshal(fr.Body.Bytes(), &fb)
			if len(fb.Findings) != 1 || fb.Findings[0].Signature != "Win.Test.EICAR_HDB-1" {
				t.Fatalf("findings = %+v", fb.Findings)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
}

func TestCreateScanRejectsBadPaths(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())
	for _, body := range []string{
		`{"paths":[]}`,
		`{"paths":["relative/path"]}`,
		`{"paths":["   "]}`,
	} {
		rec := do(t, s, http.MethodPost, "/api/scans", body, cookies, csrf)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> status %d, want 400", body, rec.Code)
		}
	}
}

func TestCancelScanConflictWhenDone(t *testing.T) {
	r := newStub()
	r.have["clamscan"] = true
	r.out["clamscan --stdout /x"] = "/x: OK\nScanned files: 1\nInfected files: 0\n"
	s, cookies, csrf := authedServer(t, r)

	rec := do(t, s, http.MethodPost, "/api/scans", `{"paths":["/x"]}`, cookies, csrf)
	var created struct {
		ScanID int64 `json:"scan_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gr := do(t, s, http.MethodGet, "/api/scans/"+strconv.FormatInt(created.ScanID, 10), "", cookies, "")
		if strings.Contains(gr.Body.String(), `"status":"done"`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	rec = do(t, s, http.MethodPost, "/api/scans/"+strconv.FormatInt(created.ScanID, 10)+"/cancel", "", cookies, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("cancel of finished scan: status = %d", rec.Code)
	}
}

func TestBrowseJail(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)

	s := newTestServer(t)
	s.cfg.BrowseRoot = filepath.ToSlash(dir)
	rec := do(t, s, http.MethodPost, "/api/setup", `{"password":"a-good-password"}`, nil, "")
	cookies := rec.Result().Cookies()

	// Listing the root works, dirs sort first.
	gr := do(t, s, http.MethodGet, "/api/browse?path="+filepath.ToSlash(dir), "", cookies, "")
	if gr.Code != http.StatusOK {
		t.Fatalf("browse root: status = %d body=%s", gr.Code, gr.Body)
	}
	var b struct {
		Parent  string `json:"parent"`
		Entries []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
		} `json:"entries"`
	}
	json.Unmarshal(gr.Body.Bytes(), &b)
	if b.Parent != "" {
		t.Errorf("root parent should be empty, got %q", b.Parent)
	}
	if len(b.Entries) != 2 || !b.Entries[0].IsDir {
		t.Fatalf("entries = %+v", b.Entries)
	}

	// Escaping the root is refused.
	gr = do(t, s, http.MethodGet, "/api/browse?path=/etc", "", cookies, "")
	if gr.Code != http.StatusForbidden {
		t.Errorf("escape attempt: status = %d, want 403", gr.Code)
	}
}
