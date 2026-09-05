package clamav

import (
	"fmt"
	"sort"
	"strings"
)

// ConfKind classifies a config value for the UI.
type ConfKind string

const (
	KindString ConfKind = "string"
	KindBool   ConfKind = "bool"
	KindInt    ConfKind = "int"
	KindPath   ConfKind = "path"
	KindSize   ConfKind = "size" // e.g. "25M"
)

// ConfKey is one editable setting in a ClamAV config file.
type ConfKey struct {
	Name       string   `json:"name"`
	Kind       ConfKind `json:"kind"`
	Repeatable bool     `json:"repeatable"`
	Help       string   `json:"help"`
}

// clamdKeys / freshclamKeys are the ONLY settings the UI may change. Everything
// else in the file is preserved verbatim on write.
var clamdKeys = []ConfKey{
	{"MaxThreads", KindInt, false, "Worker threads for the scanning daemon."},
	{"MaxFileSize", KindSize, false, "Files larger than this are skipped (e.g. 100M)."},
	{"MaxScanSize", KindSize, false, "Stop scanning an archive after this much data."},
	{"LogVerbose", KindBool, false, "Verbose daemon logging."},
	{"LogTime", KindBool, false, "Prefix log lines with a timestamp."},
	{"OnAccessMaxFileSize", KindSize, false, "On-access: skip files larger than this."},
	{"OnAccessPrevention", KindBool, false, "On-access: block access to infected files (needs fanotify)."},
	{"OnAccessIncludePath", KindPath, true, "On-access: a directory tree to watch."},
	{"OnAccessExcludePath", KindPath, true, "On-access: a path to ignore."},
	{"OnAccessExcludeUname", KindString, true, "On-access: a username whose file access is not scanned."},
}

var freshclamKeys = []ConfKey{
	{"Checks", KindInt, false, "Database update checks per day."},
	{"LogVerbose", KindBool, false, "Verbose updater logging."},
	{"NotifyClamd", KindString, false, "Path to clamd.conf to notify after an update."},
	{"DatabaseMirror", KindString, true, "A mirror host to fetch signatures from."},
}

func confKeysFor(which string) ([]ConfKey, string, string, error) {
	switch which {
	case "clamd":
		return clamdKeys, "clamav-daemon", "clamd.conf", nil
	case "freshclam":
		return freshclamKeys, "clamav-freshclam", "freshclam.conf", nil
	}
	return nil, "", "", fmt.Errorf("%w: %s", ErrUnknownConf, which)
}

func (m *Manager) confPath(which string) string {
	if which == "freshclam" {
		return m.cfg.FreshclamConf
	}
	return m.cfg.ClamdConf
}

// ConfEntry is a whitelisted key with its current value(s).
type ConfEntry struct {
	ConfKey
	Values []string `json:"values"`
}

// ConfView is a config file rendered down to its editable keys.
type ConfView struct {
	Which   string      `json:"which"`
	Path    string      `json:"path"`
	Unit    string      `json:"unit"`
	Entries []ConfEntry `json:"entries"`
}

// ReadConf returns the whitelisted settings of clamd.conf or freshclam.conf.
func (m *Manager) ReadConf(which string) (ConfView, error) {
	keys, unit, _, err := confKeysFor(which)
	if err != nil {
		return ConfView{}, err
	}
	path := m.confPath(which)

	values := map[string][]string{}
	if b, err := m.fsys.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(strings.TrimRight(line, "\r"))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			values[fields[0]] = append(values[fields[0]], strings.Join(fields[1:], " "))
		}
	}

	view := ConfView{Which: which, Path: path, Unit: unit}
	for _, k := range keys {
		view.Entries = append(view.Entries, ConfEntry{ConfKey: k, Values: values[k.Name]})
	}
	return view, nil
}

// WriteConf applies updates (whitelisted keys only) to the config file: each
// key in updates has all its existing lines removed and the new values
// appended; an empty slice removes the key. Non-whitelisted lines and comments
// are preserved. The write is atomic (temp file + rename) and keeps a .bak.
func (m *Manager) WriteConf(which string, updates map[string][]string) error {
	keys, _, _, err := confKeysFor(which)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k.Name] = true
	}
	for k := range updates {
		if !allowed[k] {
			return fmt.Errorf("%w: %s", ErrConfKeyNotAllowed, k)
		}
	}

	path := m.confPath(which)
	orig, err := m.fsys.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var out []string
	for _, line := range strings.Split(string(orig), "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			key := strings.Fields(trimmed)[0]
			if _, replacing := updates[key]; replacing {
				continue // drop; we re-add (or not) below
			}
		}
		out = append(out, strings.TrimRight(line, "\r"))
	}
	// Trim a trailing blank run, then append our keys.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}

	changed := make([]string, 0, len(updates))
	for k := range updates {
		changed = append(changed, k)
	}
	sort.Strings(changed)
	for _, k := range changed {
		for _, v := range updates[k] {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			out = append(out, k+" "+v)
		}
	}

	body := strings.Join(out, "\n") + "\n"

	// temp + rename, with a .bak of the previous content.
	if err := m.fsys.WriteFile(path+".bak", orig, 0o600); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	tmp := path + ".tmp"
	if err := m.fsys.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := m.fsys.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// confValueBool interprets a ClamAV boolean string.
func confValueBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "true", "1", "on":
		return true
	}
	return false
}
