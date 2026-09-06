package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
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
	paths, err := sanitizeScanPaths(req.Paths, s.cfg.BrowseRoot)
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
		return s.runScan(ctx, jc, scanID, "manual", paths, req.Options)
	})
	if err != nil {
		_ = s.db.FinishScan(scanID, "error", 0, 0, err.Error())
		writeError(w, http.StatusServiceUnavailable, "could not schedule scan")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"scan_id": scanID, "job_id": jobID})
}

// runScan is the worker body for a scan job.
func (s *Server) runScan(ctx context.Context, jc *worker.JobContext, scanID int64, source string, paths []string, opts clamav.ScanOptions) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()

	// A cancel that landed while the job sat in the queue.
	if cur, err := s.db.GetScan(scanID); err == nil && cur.Status == "canceled" {
		return nil
	}

	install, _ := s.clam.Detect(ctx)
	if err := s.db.StartScan(scanID, install.EngineVersion, install.DBVersion); err != nil {
		slog.Error("scan: start row", "scan", scanID, "err", err)
	}

	// Expand any disk-image targets (.iso, ...) to their contents — loop-mounted,
	// or extracted when a mount isn't permitted — so the files inside get
	// scanned rather than the opaque blob, which clamd would skip past its size
	// limits. cleanupImages is safe when nothing was prepared.
	id := strconv.FormatInt(scanID, 10)
	mcfg := s.scanMountConfig()
	mountBase := path.Join(scanMountRoot(s.cfg.ConfigDir), id)
	extractBase := path.Join(s.scanExtractRoot(mcfg), id)
	scanPaths, images, cleanupImages := s.clam.PrepareImages(
		ctx, paths, mcfg, mountBase, extractBase, func(l string) { jc.Logf("%s", l) })
	defer cleanupImages()
	if len(images) > 0 {
		opts.Recursive = true // walk the expanded trees
	}

	var lastFlush time.Time
	res, scanErr := s.clam.Scan(ctx, scanPaths, opts, clamav.ScanCallbacks{
		Line: func(line string) { jc.Logf("%s", clamav.RelabelPath(line, images)) },
		Finding: func(f clamav.ScanFinding) {
			label := clamav.RelabelPath(f.Path, images)
			if _, err := s.db.AddFinding(scanID, label, f.Signature); err != nil {
				slog.Error("scan: add finding", "scan", scanID, "err", err)
			}
			s.bus.Publish(sse.Event{Type: "scan-finding", Data: map[string]any{
				"scan_id": scanID, "path": label, "signature": f.Signature,
			}})
		},
		Progress: func(scanned, infected int) {
			if time.Since(lastFlush) < 2*time.Second {
				return
			}
			lastFlush = time.Now()
			_ = s.db.UpdateScanProgress(scanID, scanned, infected)
			s.bus.Publish(sse.Event{Type: "scan-progress", Data: map[string]any{
				"scan_id": scanID, "scanned": scanned, "infected": infected,
			}})
		},
	})

	infected := res.Infected
	if infected == 0 && len(res.Findings) > 0 {
		infected = len(res.Findings)
	}

	if scanErr != nil {
		status := "error"
		if errors.Is(scanErr, context.Canceled) || ctx.Err() != nil {
			status = "canceled"
		}
		_ = s.db.FinishScan(scanID, status, res.Scanned, infected, scanErr.Error())
		if status == "error" {
			label := "Scan"
			if source == "scheduled" {
				label = "Scheduled scan"
			}
			s.recordScanOutcome(scanID, "scan-failure", "warning",
				label+" #"+strconv.FormatInt(scanID, 10)+" failed: "+scanErr.Error())
		}
		return scanErr
	}

	_ = s.db.FinishScan(scanID, "done", res.Scanned, infected, "")
	if infected > 0 {
		s.recordScanOutcome(scanID, "scan-detection", "critical",
			"Scan #"+strconv.FormatInt(scanID, 10)+" found "+strconv.Itoa(infected)+" infected file(s).")
	}
	return nil
}

// recordScanOutcome adds an event, publishes it, and fires a notification.
func (s *Server) recordScanOutcome(scanID int64, kind, severity, msg string) {
	if _, err := s.db.AddEvent(kind, severity, msg, map[string]any{"scan_id": scanID}); err != nil {
		slog.Error("scan: add event", "err", err)
	}
	s.bus.Publish(sse.Event{Type: "event", Data: map[string]any{"kind": kind, "severity": severity, "message": msg}})
	s.notify.Dispatch(kind, msg)
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
	// Mark it now so a still-queued job bails when it dequeues; also cancel the
	// job if it is already running.
	if scan.Status == "queued" {
		_ = s.db.FinishScan(scan.ID, "canceled", 0, 0, "canceled before start")
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
// POSIX paths, confined to browseRoot, de-duplicated, at least one.
func sanitizeScanPaths(in []string, browseRoot string) ([]string, error) {
	root := path.Clean("/" + strings.TrimPrefix(filepath.ToSlash(browseRoot), "/"))
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
		clean := path.Clean(p) // resolves any ".." to a concrete absolute path
		if root != "/" && clean != root && !strings.HasPrefix(clean, root+"/") {
			return nil, errors.New("path is outside the allowed root")
		}
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one valid path is required")
	}
	if len(out) > 64 {
		return nil, errors.New("too many paths (max 64)")
	}
	return out, nil
}
