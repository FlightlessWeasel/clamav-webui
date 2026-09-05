package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestQuarantineHoldRestoreDelete(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())

	work := t.TempDir()
	s.cfg.BrowseRoot = work
	victim := filepath.Join(work, "eicar.com")
	if err := os.WriteFile(victim, []byte("bad payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"path":` + strconv.Quote(victim) + `,"signature":"Test.Sig"}`

	// Hold.
	rec := do(t, s, http.MethodPost, "/api/quarantine", body, cookies, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("hold: status = %d body=%s", rec.Code, rec.Body)
	}
	var item struct {
		ID        int64  `json:"id"`
		Status    string `json:"status"`
		OrigPath  string `json:"orig_path"`
		Signature string `json:"signature"`
	}
	json.Unmarshal(rec.Body.Bytes(), &item)
	if item.ID == 0 || item.Status != "held" || item.Signature != "Test.Sig" {
		t.Fatalf("item = %+v", item)
	}
	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Fatal("victim file should be gone after hold")
	}

	// It shows up in the list.
	lr := do(t, s, http.MethodGet, "/api/quarantine", "", cookies, "")
	var list struct {
		Items []struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	json.Unmarshal(lr.Body.Bytes(), &list)
	if len(list.Items) != 1 || list.Items[0].ID != item.ID {
		t.Fatalf("list = %+v", list.Items)
	}

	// Restore.
	rr := do(t, s, http.MethodPost, "/api/quarantine/"+strconv.FormatInt(item.ID, 10)+"/restore", "", cookies, csrf)
	if rr.Code != http.StatusOK {
		t.Fatalf("restore: status = %d body=%s", rr.Code, rr.Body)
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "bad payload" {
		t.Fatalf("restore did not put the file back: %v / %q", err, b)
	}

	// Restoring again is a conflict (no longer held).
	rr = do(t, s, http.MethodPost, "/api/quarantine/"+strconv.FormatInt(item.ID, 10)+"/restore", "", cookies, csrf)
	if rr.Code != http.StatusConflict {
		t.Errorf("second restore: status = %d, want 409", rr.Code)
	}
}

func TestQuarantineDeleteRemovesBlob(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())
	work := t.TempDir()
	s.cfg.BrowseRoot = work
	victim := filepath.Join(work, "f")
	os.WriteFile(victim, []byte("payload"), 0o644)

	rec := do(t, s, http.MethodPost, "/api/quarantine",
		`{"path":`+strconv.Quote(victim)+`,"signature":"S"}`, cookies, csrf)
	var item struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &item)

	dr := do(t, s, http.MethodPost, "/api/quarantine/"+strconv.FormatInt(item.ID, 10)+"/delete", "", cookies, csrf)
	if dr.Code != http.StatusOK {
		t.Fatalf("delete: status = %d body=%s", dr.Code, dr.Body)
	}
	var after struct {
		Status string `json:"status"`
	}
	json.Unmarshal(dr.Body.Bytes(), &after)
	if after.Status != "deleted" {
		t.Errorf("status after delete = %q", after.Status)
	}
	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Error("original path should still be empty after delete")
	}
}

func TestQuarantineRejectsRelativePath(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())
	rec := do(t, s, http.MethodPost, "/api/quarantine", `{"path":"relative/x","signature":"S"}`, cookies, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestQuarantineRefusesConfinedPaths(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())
	root := t.TempDir()
	s.cfg.BrowseRoot = root

	// Inside the app's data directory.
	inCfg := strconv.Quote(filepath.Join(s.cfg.ConfigDir, "clamav-webui.db"))
	rec := do(t, s, http.MethodPost, "/api/quarantine", `{"path":`+inCfg+`,"signature":"S"}`, cookies, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("data-dir path: status = %d, want 400", rec.Code)
	}

	// Outside the browse root.
	outside := strconv.Quote(filepath.Join(t.TempDir(), "elsewhere"))
	rec = do(t, s, http.MethodPost, "/api/quarantine", `{"path":`+outside+`,"signature":"S"}`, cookies, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("outside-root path: status = %d, want 400", rec.Code)
	}
}
