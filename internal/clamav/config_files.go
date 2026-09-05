package clamav

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
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

var sizeRE = regexp.MustCompile(`(?i)^\d+[kmg]?$`)

// validateConfValue checks a single value against a key's declared kind.
func validateConfValue(k ConfKey, v string) error {
	switch k.Kind {
	case KindInt:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("%w: %s must be an integer, got %q", ErrConfValueInvalid, k.Name, v)
		}
	case KindSize:
		if !sizeRE.MatchString(v) {
			return fmt.Errorf("%w: %s must be a size like 100M, got %q", ErrConfValueInvalid, k.Name, v)
		}
	case KindPath:
		if !strings.HasPrefix(v, "/") {
			return fmt.Errorf("%w: %s must be an absolute path, got %q", ErrConfValueInvalid, k.Name, v)
		}
	}
	return nil
}

// normalizeConfValue canonicalises a value for writing (booleans -> yes/no).
func normalizeConfValue(k ConfKey, v string) string {
	if k.Kind == KindBool {
		if confValueBool(v) {
			return "yes"
		}
		return "no"
	}
	return v
}

// WriteConf applies updates (whitelisted keys only) to the config file. Each
// updated key's first line is rewritten in place with the new value(s) and any
// further lines for it are dropped; an empty value set removes the key; a key
// not already present is appended. Non-whitelisted lines and comments are
// untouched. The write is atomic (same-dir temp + rename), preserves the
// file's mode, and keeps a .bak.
func (m *Manager) WriteConf(which string, updates map[string][]string) error {
	keys, _, _, err := confKeysFor(which)
	if err != nil {
		return err
	}
	byName := map[string]ConfKey{}
	for _, k := range keys {
		byName[k.Name] = k
	}

	// Validate keys and values up front; write nothing on any error.
	cleaned := map[string][]string{}
	for name, vals := range updates {
		k, ok := byName[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrConfKeyNotAllowed, name)
		}
		var kept []string
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if err := validateConfValue(k, v); err != nil {
				return err
			}
			kept = append(kept, normalizeConfValue(k, v))
		}
		cleaned[name] = kept
	}

	m.confMu.Lock()
	defer m.confMu.Unlock()

	path := m.confPath(which)
	orig, err := m.fsys.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	mode := os.FileMode(0o644)
	if fi, err := m.fsys.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}

	written := map[string]bool{}
	var out []string
	for _, raw := range strings.Split(string(orig), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		key := ""
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			key = strings.Fields(trimmed)[0]
		}
		vals, replacing := cleaned[key]
		if !replacing {
			out = append(out, line)
			continue
		}
		if written[key] {
			continue // subsequent line for an already-rewritten key: drop
		}
		written[key] = true
		for _, v := range vals {
			out = append(out, key+" "+v)
		}
	}

	// Keys that weren't already in the file get appended.
	for name, vals := range cleaned {
		if written[name] || len(vals) == 0 {
			continue
		}
		for _, v := range vals {
			out = append(out, name+" "+v)
		}
	}

	body := strings.Join(out, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}

	if err := m.fsys.WriteFile(path+".bak", orig, mode); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	tmp := path + ".tmp"
	if err := m.fsys.WriteFile(tmp, []byte(body), mode); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := m.fsys.Rename(tmp, path); err != nil {
		_ = m.fsys.Remove(tmp)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// RestoreConfBackup rolls a config file back to its .bak (best-effort).
func (m *Manager) RestoreConfBackup(which string) error {
	path := m.confPath(which)
	m.confMu.Lock()
	defer m.confMu.Unlock()
	return m.fsys.Rename(path+".bak", path)
}

// confValueBool interprets a ClamAV boolean string.
func confValueBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "true", "1", "on":
		return true
	}
	return false
}
