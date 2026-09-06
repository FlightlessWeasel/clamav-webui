package api

import (
	"encoding/json"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanMountConfigRoundTrip(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())

	// Default: disabled, with the built-in extension list, never JSON null.
	gr := do(t, s, http.MethodGet, "/api/scan-mount", "", cookies, "")
	if gr.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", gr.Code, gr.Body)
	}
	if strings.Contains(gr.Body.String(), `"extensions":null`) {
		t.Fatalf("null array leaked: %s", gr.Body)
	}
	if !strings.Contains(gr.Body.String(), `"enabled":false`) {
		t.Fatalf("want disabled by default: %s", gr.Body)
	}

	pr := do(t, s, http.MethodPut, "/api/scan-mount",
		`{"enabled":true,"extensions":[".iso",".img"]}`, cookies, csrf)
	if pr.Code != http.StatusOK {
		t.Fatalf("put: %d body=%s", pr.Code, pr.Body)
	}
	var got struct {
		Enabled    bool     `json:"enabled"`
		Extensions []string `json:"extensions"`
	}
	json.Unmarshal(do(t, s, http.MethodGet, "/api/scan-mount", "", cookies, "").Body.Bytes(), &got)
	if !got.Enabled || len(got.Extensions) != 2 {
		t.Fatalf("not persisted: %+v", got)
	}
}

// A scan of an .iso target, with mounting enabled, must loop-mount the image,
// scan the mountpoint instead of the raw file, relabel findings back to
// "image!/inner", and unmount afterwards.
func TestScanMountsDiskImage(t *testing.T) {
	r := newStub()
	r.have["clamscan"] = true

	s, cookies, csrf := authedServer(t, r)
	if pr := do(t, s, http.MethodPut, "/api/scan-mount",
		`{"enabled":true,"extensions":[".iso"]}`, cookies, csrf); pr.Code != http.StatusOK {
		t.Fatalf("enable: %d", pr.Code)
	}

	// First scan in a fresh DB is id 1, so the mountpoint is deterministic.
	mp := path.Join(filepath.ToSlash(s.cfg.ConfigDir), "mnt", "1", "0")
	r.out["mount -o loop,ro,nodev,nosuid,noexec /srv/x.iso "+mp] = ""
	r.out["umount "+mp] = ""
	r.out["clamscan --stdout --recursive "+mp] = strings.Join([]string{
		mp + "/clean.bin: OK",
		mp + "/evil.exe: Win.Test.EICAR_HDB-1 FOUND",
		"----------- SCAN SUMMARY -----------",
		"Engine version: 1.0.3",
		"Scanned files: 2",
		"Infected files: 1",
	}, "\n")
	r.exit["clamscan --stdout --recursive "+mp] = 1

	rec := do(t, s, http.MethodPost, "/api/scans",
		`{"paths":["/srv/x.iso"],"options":{"force_clamscan":true}}`, cookies, csrf)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create scan: %d body=%s", rec.Code, rec.Body)
	}
	var created struct {
		ScanID int64 `json:"scan_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ScanID != 1 {
		t.Fatalf("scan id = %d, want 1", created.ScanID)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gr := do(t, s, http.MethodGet, "/api/scans/1", "", cookies, "")
		if strings.Contains(gr.Body.String(), `"status":"done"`) {
			fr := do(t, s, http.MethodGet, "/api/scans/1/findings", "", cookies, "")
			var fb struct {
				Findings []struct {
					Path      string `json:"path"`
					Signature string `json:"signature"`
				} `json:"findings"`
			}
			json.Unmarshal(fr.Body.Bytes(), &fb)
			if len(fb.Findings) != 1 || fb.Findings[0].Path != "/srv/x.iso!/evil.exe" {
				t.Fatalf("findings = %+v, want path relabeled", fb.Findings)
			}
			if !r.saw("mount -o loop,ro,nodev,nosuid,noexec /srv/x.iso " + mp) {
				t.Errorf("image was not mounted")
			}
			if !r.saw("umount " + mp) {
				t.Errorf("image was not unmounted")
			}
			if r.saw("clamscan --stdout --recursive /srv/x.iso") {
				t.Errorf("raw image was scanned instead of the mountpoint")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
}
