package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/FlightlessWeasel/clamav-webui/internal/clamav"
	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/FlightlessWeasel/clamav-webui/internal/scheduler"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
	"github.com/FlightlessWeasel/clamav-webui/internal/worker"
	"github.com/go-chi/chi/v5"
)

type scheduleRequest struct {
	Name    string             `json:"name"`
	Cron    string             `json:"cron_expr"`
	Paths   []string           `json:"paths"`
	Options clamav.ScanOptions `json:"options"`
	Enabled bool               `json:"enabled"`
}

func (r scheduleRequest) toInput(paths []string) db.ScheduleInput {
	return db.ScheduleInput{
		Name:     strings.TrimSpace(r.Name),
		CronExpr: strings.TrimSpace(r.Cron),
		Paths:    paths,
		Options:  r.Options,
		Enabled:  r.Enabled,
	}
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListSchedules()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": list})
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	req, paths, ok := s.decodeSchedule(w, r)
	if !ok {
		return
	}
	id, err := s.db.CreateSchedule(req.toInput(paths))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.sched.Reload(); err != nil {
		slog.Error("schedule: reload", "err", err)
	}
	sc, _ := s.db.GetSchedule(id)
	writeJSON(w, http.StatusCreated, sc)
}

func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	req, paths, ok := s.decodeSchedule(w, r)
	if !ok {
		return
	}
	if err := s.db.UpdateSchedule(id, req.toInput(paths)); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.sched.Reload(); err != nil {
		slog.Error("schedule: reload", "err", err)
	}
	sc, _ := s.db.GetSchedule(id)
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := s.db.DeleteSchedule(id); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.sched.Reload(); err != nil {
		slog.Error("schedule: reload", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if _, err := s.db.GetSchedule(id); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.runScheduledScan(id)
	writeJSON(w, http.StatusAccepted, map[string]string{"result": "started"})
}

// decodeSchedule parses and validates a schedule request body.
func (s *Server) decodeSchedule(w http.ResponseWriter, r *http.Request) (scheduleRequest, []string, bool) {
	var req scheduleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return req, nil, false
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return req, nil, false
	}
	if err := scheduler.ValidateSpec(strings.TrimSpace(req.Cron)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron expression: "+err.Error())
		return req, nil, false
	}
	paths, err := sanitizeScanPaths(req.Paths, s.cfg.BrowseRoot)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, nil, false
	}
	return req, paths, true
}

// runScheduledScan is the scheduler callback: create a scan row for the
// schedule and enqueue it.
func (s *Server) runScheduledScan(scheduleID int64) {
	sch, err := s.db.GetSchedule(scheduleID)
	if err != nil {
		slog.Error("scheduled scan: load", "schedule", scheduleID, "err", err)
		return
	}
	// Skip this run if the schedule's previous scan hasn't finished, so a
	// tight cron over a slow scan doesn't pile up rows and jobs.
	if sch.LastRunID != 0 {
		if prev, err := s.db.GetScan(sch.LastRunID); err == nil &&
			(prev.Status == "queued" || prev.Status == "running") {
			slog.Info("scheduled scan: previous run still active, skipping", "schedule", scheduleID, "scan", prev.ID)
			return
		}
	}

	var opts clamav.ScanOptions
	_ = json.Unmarshal(sch.Options, &opts)

	scanID, err := s.db.CreateScan("scheduled", sch.Paths, opts)
	if err != nil {
		slog.Error("scheduled scan: create row", "schedule", scheduleID, "err", err)
		return
	}
	_, err = s.jobs.Enqueue("scan", &scanID, func(ctx context.Context, jc *worker.JobContext) error {
		return s.runScan(ctx, jc, scanID, "scheduled", sch.Paths, opts)
	})
	if err != nil {
		_ = s.db.FinishScan(scanID, "error", 0, 0, err.Error())
		return
	}
	_ = s.db.SetScheduleLastRun(scheduleID, scanID)
	s.bus.Publish(sse.Event{Type: "schedule", Data: map[string]any{"schedule_id": scheduleID, "scan_id": scanID}})
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad id")
		return 0, false
	}
	return id, true
}
