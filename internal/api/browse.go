package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type browseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

const maxBrowseEntries = 2000

// handleBrowse lists a directory for the path picker. The requested path is
// cleaned and confined to cfg.BrowseRoot; ".." cannot escape it. Paths are
// returned slash-style regardless of host OS.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	root := filepath.Clean(s.cfg.BrowseRoot)
	req := r.URL.Query().Get("path")
	if strings.TrimSpace(req) == "" {
		req = root
	}
	dir := filepath.Clean(req)

	if !filepath.IsAbs(dir) || !withinRoot(dir, root) {
		writeError(w, http.StatusForbidden, "path is outside the allowed root")
		return
	}

	info, err := os.Stat(dir)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such directory")
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not a directory")
		return
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		writeError(w, http.StatusForbidden, "cannot read directory")
		return
	}

	entries := make([]browseEntry, 0, len(items))
	for _, it := range items {
		if len(entries) >= maxBrowseEntries {
			break
		}
		entries = append(entries, browseEntry{
			Name:  it.Name(),
			Path:  filepath.ToSlash(filepath.Join(dir, it.Name())),
			IsDir: it.IsDir(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := ""
	if dir != root {
		parent = filepath.ToSlash(filepath.Dir(dir))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    filepath.ToSlash(dir),
		"parent":  parent,
		"root":    filepath.ToSlash(root),
		"entries": entries,
	})
}

// withinRoot reports whether dir is root or a descendant of it.
func withinRoot(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
