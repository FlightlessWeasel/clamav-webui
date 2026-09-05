package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScheduleCRUD(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())

	// Create.
	body := `{"name":"Nightly /srv","cron_expr":"0 3 * * *","paths":["/srv"],"options":{"recursive":true},"enabled":true}`
	rec := do(t, s, http.MethodPost, "/api/schedules", body, cookies, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d body=%s", rec.Code, rec.Body)
	}
	var sc struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	json.Unmarshal(rec.Body.Bytes(), &sc)
	if sc.ID == 0 || sc.Name != "Nightly /srv" || !sc.Enabled {
		t.Fatalf("created = %+v", sc)
	}

	// List.
	lr := do(t, s, http.MethodGet, "/api/schedules", "", cookies, "")
	var list struct {
		Schedules []struct {
			ID int64 `json:"id"`
		} `json:"schedules"`
	}
	json.Unmarshal(lr.Body.Bytes(), &list)
	if len(list.Schedules) != 1 {
		t.Fatalf("list len = %d", len(list.Schedules))
	}

	// Update (disable).
	up := `{"name":"Nightly /srv","cron_expr":"30 2 * * *","paths":["/srv"],"options":{},"enabled":false}`
	ur := do(t, s, http.MethodPut, "/api/schedules/"+strconv.FormatInt(sc.ID, 10), up, cookies, csrf)
	if ur.Code != http.StatusOK {
		t.Fatalf("update: status = %d body=%s", ur.Code, ur.Body)
	}

	// Delete.
	dr := do(t, s, http.MethodDelete, "/api/schedules/"+strconv.FormatInt(sc.ID, 10), "", cookies, csrf)
	if dr.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d", dr.Code)
	}
	lr = do(t, s, http.MethodGet, "/api/schedules", "", cookies, "")
	json.Unmarshal(lr.Body.Bytes(), &list)
	if len(list.Schedules) != 0 {
		t.Fatalf("list after delete = %d", len(list.Schedules))
	}
}

func TestScheduleValidation(t *testing.T) {
	s, cookies, csrf := authedServer(t, newStub())
	cases := []string{
		`{"name":"","cron_expr":"0 3 * * *","paths":["/srv"]}`,      // no name
		`{"name":"x","cron_expr":"not cron","paths":["/srv"]}`,      // bad cron
		`{"name":"x","cron_expr":"0 3 * * *","paths":[]}`,           // no paths
		`{"name":"x","cron_expr":"0 3 * * *","paths":["relative"]}`, // non-absolute
	}
	for _, c := range cases {
		rec := do(t, s, http.MethodPost, "/api/schedules", c, cookies, csrf)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", c, rec.Code)
		}
	}
}

func TestScheduleRunNow(t *testing.T) {
	r := newStub()
	r.have["clamscan"] = true
	r.out["clamscan --version"] = "ClamAV 1.0.3/27000/x"
	r.out["clamscan --stdout /data"] = "/data/x: OK\nScanned files: 1\nInfected files: 0\n"
	s, cookies, csrf := authedServer(t, r)

	cr := do(t, s, http.MethodPost, "/api/schedules",
		`{"name":"m","cron_expr":"0 0 1 1 *","paths":["/data"],"options":{},"enabled":true}`, cookies, csrf)
	var sc struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(cr.Body.Bytes(), &sc)

	rr := do(t, s, http.MethodPost, "/api/schedules/"+strconv.FormatInt(sc.ID, 10)+"/run", "", cookies, csrf)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("run: status = %d body=%s", rr.Code, rr.Body)
	}

	// A scheduled scan row should appear and finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lr := do(t, s, http.MethodGet, "/api/scans", "", cookies, "")
		if strings.Contains(lr.Body.String(), `"source":"scheduled"`) && strings.Contains(lr.Body.String(), `"status":"done"`) {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("scheduled scan did not run to completion")
}
