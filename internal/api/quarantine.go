package api

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/FlightlessWeasel/clamav-webui/internal/db"
	"github.com/FlightlessWeasel/clamav-webui/internal/quarantine"
	"github.com/FlightlessWeasel/clamav-webui/internal/sse"
	"github.com/go-chi/chi/v5"
)

type quarantineRequest struct {
	Path      string `json:"path"`
	Signature string `json:"signature"`
	ScanID    int64  `json:"scan_id"`
	FindingID int64  `json:"finding_id"`
}

func (s *Server) handleListQuarantine(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListQuarantine(false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleQuarantineFile moves an infected file into the store and records it.
func (s *Server) handleQuarantineFile(w http.ResponseWriter, r *http.Request) {
	var req quarantineRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, jsonErrMsg(err))
		return
	}
	if !filepath.IsAbs(req.Path) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}

	sc, err := s.qstore.Hold(req.Path, req.Signature)
	if err != nil {
		slog.Error("quarantine: hold", "path", req.Path, "err", err)
		writeError(w, http.StatusBadGateway, "could not quarantine file: "+err.Error())
		return
	}

	var scanID *int64
	if req.ScanID > 0 {
		scanID = &req.ScanID
	}
	id, err := s.db.AddQuarantine(sc.Name, sc.OrigPath, sc.Signature, sc.SHA256, scanID, sc.OrigMode)
	if err != nil {
		slog.Error("quarantine: record", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if req.FindingID > 0 {
		if err := s.db.SetFindingAction(req.FindingID, "quarantined"); err != nil {
			slog.Error("quarantine: mark finding", "finding", req.FindingID, "err", err)
		}
	}

	s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": id, "action": "held", "path": sc.OrigPath}})
	item, _ := s.db.GetQuarantine(id)
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleRestoreQuarantine(w http.ResponseWriter, r *http.Request) {
	item, ok := s.quarantineFromURL(w, r)
	if !ok {
		return
	}
	if item.Status != "held" {
		writeError(w, http.StatusConflict, "item is not currently held")
		return
	}
	err := s.qstore.Restore(item.StoreName, item.OrigPath, item.OrigMode)
	if errors.Is(err, quarantine.ErrOriginExists) {
		writeError(w, http.StatusConflict, "a file already exists at the original path")
		return
	}
	if err != nil {
		slog.Error("quarantine: restore", "id", item.ID, "err", err)
		writeError(w, http.StatusBadGateway, "restore failed: "+err.Error())
		return
	}
	_ = s.db.SetQuarantineStatus(item.ID, "restored")
	s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": item.ID, "action": "restored"}})
	next, _ := s.db.GetQuarantine(item.ID)
	writeJSON(w, http.StatusOK, next)
}

func (s *Server) handleDeleteQuarantine(w http.ResponseWriter, r *http.Request) {
	item, ok := s.quarantineFromURL(w, r)
	if !ok {
		return
	}
	if item.Status == "held" {
		if err := s.qstore.Purge(item.StoreName); err != nil {
			slog.Error("quarantine: purge", "id", item.ID, "err", err)
			writeError(w, http.StatusBadGateway, "delete failed: "+err.Error())
			return
		}
	}
	_ = s.db.SetQuarantineStatus(item.ID, "deleted")
	s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": item.ID, "action": "deleted"}})
	next, _ := s.db.GetQuarantine(item.ID)
	writeJSON(w, http.StatusOK, next)
}

func (s *Server) quarantineFromURL(w http.ResponseWriter, r *http.Request) (db.QuarantineItem, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad id")
		return db.QuarantineItem{}, false
	}
	item, err := s.db.GetQuarantine(id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return db.QuarantineItem{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return db.QuarantineItem{}, false
	}
	return item, true
}
