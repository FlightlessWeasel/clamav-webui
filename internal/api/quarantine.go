package api

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

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
	items, err := s.db.ListQuarantine()
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
	if err := s.checkTargetPath(req.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sc, err := s.qstore.Hold(req.Path, req.Signature, req.ScanID)
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
		// Don't lose the file: put it back and report failure.
		slog.Error("quarantine: record failed, restoring file", "store_name", sc.Name, "orig", sc.OrigPath, "err", err)
		if rerr := s.qstore.Restore(sc); rerr != nil {
			slog.Error("quarantine: restore after failed record ALSO failed", "store_name", sc.Name, "err", rerr)
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if req.FindingID > 0 {
		if err := s.db.SetFindingAction(req.FindingID, "quarantined"); err != nil {
			slog.Error("quarantine: mark finding", "finding", req.FindingID, "err", err)
		}
	}

	s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": id, "action": "held", "path": sc.OrigPath}})
	item, err := s.db.GetQuarantine(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
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

	err := s.qstore.Restore(s.sidecarFromItem(item))
	switch {
	case errors.Is(err, quarantine.ErrOriginExists):
		writeError(w, http.StatusConflict, "a file already exists at the original path")
		return
	case errors.Is(err, quarantine.ErrOriginParentGone):
		writeError(w, http.StatusConflict, "the original directory no longer exists")
		return
	case err != nil:
		slog.Error("quarantine: restore", "id", item.ID, "err", err)
		writeError(w, http.StatusBadGateway, "restore failed: "+err.Error())
		return
	}

	if err := s.db.SetQuarantineStatus(item.ID, "restored"); err != nil {
		slog.Error("quarantine: status after restore", "id", item.ID, "err", err)
	}
	s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": item.ID, "action": "restored"}})
	s.writeQuarantineItem(w, item.ID)
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
	if err := s.db.SetQuarantineStatus(item.ID, "deleted"); err != nil {
		slog.Error("quarantine: status after delete", "id", item.ID, "err", err)
	}
	s.bus.Publish(sse.Event{Type: "quarantine", Data: map[string]any{"id": item.ID, "action": "deleted"}})
	s.writeQuarantineItem(w, item.ID)
}

// checkTargetPath refuses relative paths, anything outside the browse root, and
// the app's own state directory. It guards both quarantine targets and
// on-access watch paths.
func (s *Server) checkTargetPath(p string) error {
	if !filepath.IsAbs(p) && !strings.HasPrefix(p, "/") {
		return errors.New("path must be absolute")
	}
	clean := filepath.Clean(p)
	if withinRoot(clean, filepath.Clean(s.cfg.ConfigDir)) {
		return errors.New("path is inside the application's data directory")
	}
	if !withinRoot(clean, filepath.Clean(s.cfg.BrowseRoot)) {
		return errors.New("path is outside the allowed root")
	}
	return nil
}

func (s *Server) sidecarFromItem(it db.QuarantineItem) quarantine.Sidecar {
	if sc, err := s.qstore.SidecarFor(it.StoreName); err == nil {
		return sc
	}
	return quarantine.Sidecar{
		Name: it.StoreName, OrigPath: it.OrigPath, OrigMode: it.OrigMode,
		Signature: it.Signature, SHA256: it.SHA256, ScanID: it.ScanID,
	}
}

func (s *Server) writeQuarantineItem(w http.ResponseWriter, id int64) {
	item, err := s.db.GetQuarantine(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, item)
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
