package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
	"github.com/FlightlessWeasel/clamav-webui/internal/worker"
	"github.com/go-chi/chi/v5"
)

type createScanRequest struct {
	Paths   []string           `json:"paths"`
	Options clamav.ScanOptions `json:"options"`
}

func (s *Server) handleCreateScan(w http.ResponseWriter, r *http.Request) {
	var req createScanRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}
	paths, err := sanitizeScanPaths(req.Paths)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	scanID, err := s.db.CreateScan("manual", paths, req.Options)
	if err != nil {
		slog.Error("scan: create row", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	jobID, err := s.jobs.Enqueue("scan", &scanID, func(ctx context.Context, jc *worker.JobContext) error {
		return s.runScan(ctx, jc, scanID, paths, req.Options)
	})
	if err != nil {
		_ = s.db.FinishScan(scanID, "error", 0, 0, err.Error())
		writeError(w, http.StatusServiceUnavailable, "could not schedule scan")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"scan_id": scanID, "job_id": jobID})
}

// runScan is the worker body for a scan job.
func (s *Server) runScan(ctx context.Context, jc *worker.JobContext, scanID int64, paths []string, opts clamav.ScanOptions) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()

	install, _ := s.clam.Detect(ctx)
	if err := s.db.StartScan(scanID, install.EngineVersion, install.DBVersion); err != nil {
		slog.Error("scan: start row", "scan", scanID, "err", err)
	}

	res, scanErr := s.clam.Scan(ctx, paths, opts,
		func(line string) { jc.Logf("%s", line) },
		func(f clamav.ScanFinding) {
			if _, err := s.db.AddFinding(scanID, f.Path, f.Signature); err != nil {
				slog.Error("scan: add finding", "scan", scanID, "err", err)
			}
			s.bus.Publish(sse.Event{Type: "scan-finding", Data: map[string]any{
				"scan_id": scanID, "path": f.Path, "signature": f.Signature,
			}})
		},
	)

	if scanErr != nil {
		status := "error"
		if errors.Is(scanErr, context.Canceled) || ctx.Err() == context.Canceled {
			status = "canceled"
		}
		_ = s.db.FinishScan(scanID, status, res.Scanned, len(res.Findings), scanErr.Error())
		return scanErr
	}

	_ = s.db.FinishScan(scanID, "done", res.Scanned, res.Infected, "")
	return nil
}

func (s *Server) handleListScans(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	scans, err := s.db.ListScans(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": scans})
}

func (s *Server) handleGetScan(w http.ResponseWriter, r *http.Request) {
	scan, err := s.scanFromURL(w, r)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

func (s *Server) handleScanFindings(w http.ResponseWriter, r *http.Request) {
	scan, err := s.scanFromURL(w, r)
	if err != nil {
		return
	}
	findings, err := s.db.ScanFindings(scan.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": findings})
}

func (s *Server) handleCancelScan(w http.ResponseWriter, r *http.Request) {
	scan, err := s.scanFromURL(w, r)
	if err != nil {
		return
	}
	if scan.Status != "running" && scan.Status != "queued" {
		writeError(w, http.StatusConflict, "scan is not running")
		return
	}
	if jobID, ok, _ := s.db.RunningJobForRef("scan", scan.ID); ok {
		s.jobs.Cancel(jobID)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"result": "canceling"})
}

func (s *Server) scanFromURL(w http.ResponseWriter, r *http.Request) (db.Scan, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad scan id")
		return db.Scan{}, err
	}
	scan, err := s.db.GetScan(id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "scan not found")
		return db.Scan{}, err
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return db.Scan{}, err
	}
	return scan, nil
}

// sanitizeScanPaths cleans and validates the requested scan targets: absolute
// POSIX paths, no "..", de-duplicated, at least one.
func sanitizeScanPaths(in []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			return nil, errors.New("paths must be absolute")
		}
		clean := path.Clean(p)
		if clean == "." || strings.Contains(clean, "..") {
			return nil, errors.New("invalid path")
		}
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one path is required")
	}
	if len(out) > 64 {
		return nil, errors.New("too many paths (max 64)")
	}
	return out, nil
}
