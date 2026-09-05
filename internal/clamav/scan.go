package clamav

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ScanOptions are the user-tunable knobs for a scan. Zero values mean "ClamAV
// default". Size limits only apply to the clamscan fallback; clamd enforces its
// own configured limits.
type ScanOptions struct {
	Recursive      bool `json:"recursive"`
	FollowSymlinks bool `json:"follow_symlinks"`
	MaxFileSizeMB  int  `json:"max_file_size_mb"`
	MaxScanSizeMB  int  `json:"max_scan_size_mb"`
	// ForceClamscan skips the daemon even when it is available.
	ForceClamscan bool `json:"force_clamscan"`
}

// ScanFinding is one infected file.
type ScanFinding struct {
	Path      string `json:"path"`
	Signature string `json:"signature"`
}

// ScanResult is the parsed outcome of a completed scan.
type ScanResult struct {
	UsedDaemon    bool          `json:"used_daemon"`
	EngineVersion string        `json:"engine_version"`
	Scanned       int           `json:"scanned"`
	Infected      int           `json:"infected"`
	Findings      []ScanFinding `json:"findings"`

	// resultLines counts "<path>: OK|FOUND" lines, a fallback scanned count
	// for clamdscan (whose summary omits "Scanned files").
	resultLines int
}

var (
	foundRE    = regexp.MustCompile(`^(.*): (.+) FOUND$`)
	okRE       = regexp.MustCompile(`: OK$`)
	summaryInt = regexp.MustCompile(`^(Infected files|Scanned files|Total errors):\s*(\d+)`)
	engineRE   = regexp.MustCompile(`^Engine version:\s*(\S+)`)
)

// ScanCallbacks are the optional progress hooks for a scan. Any field may be
// nil.
type ScanCallbacks struct {
	// Line receives every raw output line, including the leading "$ cmd ..."
	// echo.
	Line func(string)
	// Finding is called once per infected file as it is discovered.
	Finding func(ScanFinding)
	// Progress is called as running totals change (scanned files, infected so
	// far) so the caller can persist/stream interim state.
	Progress func(scanned, infected int)
}

// Scan scans paths, reporting progress through cb. A "virus found" exit
// (clamscan/clamdscan code 1) is not treated as an error.
func (m *Manager) Scan(ctx context.Context, paths []string, opts ScanOptions, cb ScanCallbacks) (ScanResult, error) {
	if cb.Line == nil {
		cb.Line = func(string) {}
	}
	if cb.Finding == nil {
		cb.Finding = func(ScanFinding) {}
	}
	if cb.Progress == nil {
		cb.Progress = func(int, int) {}
	}
	if len(paths) == 0 {
		return ScanResult{}, errors.New("clamav: no paths to scan")
	}
	if _, ok := m.run.LookPath("clamscan"); !ok {
		if _, ok := m.run.LookPath("clamdscan"); !ok {
			return ScanResult{}, ErrNotInstalled
		}
	}

	cmd, usedDaemon := m.buildScanCmd(ctx, paths, opts)
	cb.Line("$ " + cmd.Name + " " + strings.Join(cmd.Args, " "))

	var res ScanResult
	res.UsedDaemon = usedDaemon

	streamErr := m.run.Stream(ctx, cmd, func(line string) {
		cb.Line(line)
		before := res.resultLines
		parseScanLine(line, &res, cb.Finding)
		if res.resultLines != before {
			cb.Progress(res.resultLines, len(res.Findings))
		}
	})

	// clamscan / clamdscan: 0 = clean, 1 = infection(s) found, >=2 = error.
	var exitErr *ExitError
	if errors.As(streamErr, &exitErr) {
		if exitErr.Code == 1 {
			streamErr = nil
		}
	}
	if streamErr != nil {
		return res, streamErr
	}

	// clamdscan's summary omits both counts; fall back to what we saw stream by.
	if res.Infected == 0 && len(res.Findings) > 0 {
		res.Infected = len(res.Findings)
	}
	if res.Scanned == 0 {
		res.Scanned = res.resultLines
	}
	return res, nil
}

// buildScanCmd chooses clamdscan (fast, DB resident) when the daemon is up,
// else clamscan, and assembles the argument list.
func (m *Manager) buildScanCmd(ctx context.Context, paths []string, opts ScanOptions) (Cmd, bool) {
	_, haveClamd := m.run.LookPath("clamdscan")
	daemonUp := false
	if haveClamd && !opts.ForceClamscan {
		if svc, err := m.Service(ctx, "clamav-daemon"); err == nil && svc.Active == "active" {
			daemonUp = true
		}
	}

	if daemonUp {
		args := []string{"--fdpass", "--multiscan", "--stdout"}
		args = append(args, paths...)
		return Cmd{Name: "clamdscan", Args: args}, true
	}

	args := []string{"--stdout"}
	if opts.Recursive {
		args = append(args, "--recursive")
	}
	if opts.FollowSymlinks {
		args = append(args, "--follow-dir-symlinks=2", "--follow-file-symlinks=2")
	}
	if opts.MaxFileSizeMB > 0 {
		args = append(args, "--max-filesize="+strconv.Itoa(opts.MaxFileSizeMB)+"M")
	}
	if opts.MaxScanSizeMB > 0 {
		args = append(args, "--max-scansize="+strconv.Itoa(opts.MaxScanSizeMB)+"M")
	}
	args = append(args, paths...)
	return Cmd{Name: "clamscan", Args: args}, false
}

func parseScanLine(line string, res *ScanResult, onFinding func(ScanFinding)) {
	line = strings.TrimRight(line, "\r")

	if m := foundRE.FindStringSubmatch(line); m != nil {
		f := ScanFinding{Path: m[1], Signature: m[2]}
		res.Findings = append(res.Findings, f)
		res.resultLines++
		onFinding(f)
		return
	}
	if okRE.MatchString(line) {
		res.resultLines++
		return
	}
	if m := summaryInt.FindStringSubmatch(line); m != nil {
		n, _ := strconv.Atoi(m[2])
		switch m[1] {
		case "Infected files":
			res.Infected = n
		case "Scanned files":
			res.Scanned = n
		}
		return
	}
	if m := engineRE.FindStringSubmatch(line); m != nil {
		res.EngineVersion = m[1]
	}
}
