package clamav

import (
	"context"
	"regexp"
	"strings"
)

// Install describes what of the ClamAV toolchain is present and whether an apt
// upgrade is available.
type Install struct {
	Installed        bool   `json:"installed"`         // clamscan is on PATH
	EngineVersion    string `json:"engine_version"`    // e.g. "1.0.3"
	DBVersion        string `json:"db_version"`        // daily signature DB number, "" if none
	DBDate           string `json:"db_date"`           // human date from clamscan --version, "" if none
	HasDaemon        bool   `json:"has_daemon"`        // clamdscan present
	HasFreshclam     bool   `json:"has_freshclam"`     // freshclam present
	HasClamonacc     bool   `json:"has_clamonacc"`     // clamonacc present
	AptInstalled     string `json:"apt_installed"`     // apt package version, "" if none
	AptCandidate     string `json:"apt_candidate"`     // apt candidate version, "" if unknown
	UpgradeAvailable bool   `json:"upgrade_available"` // candidate newer than installed
}

var versionRE = regexp.MustCompile(`ClamAV (\S+?)(?:/(\d+)/(.+))?\s*$`)

// Detect probes the toolchain. It never returns an error for "not installed";
// err is non-nil only when a probe that should have worked failed unexpectedly.
func (m *Manager) Detect(ctx context.Context) (Install, error) {
	var in Install

	_, in.Installed = m.run.LookPath("clamscan")
	_, in.HasDaemon = m.run.LookPath("clamdscan")
	_, in.HasFreshclam = m.run.LookPath("freshclam")
	_, in.HasClamonacc = m.run.LookPath("clamonacc")

	if in.Installed {
		if res, err := m.run.Run(ctx, Cmd{Name: "clamscan", Args: []string{"--version"}}); err == nil {
			parseClamVersion(strings.TrimSpace(string(res.Stdout)), &in)
		}
	}

	if pol, err := m.run.Run(ctx, Cmd{Name: "apt-cache", Args: []string{"policy", "clamav"}}); err == nil {
		inst, cand := parseAptPolicy(string(pol.Stdout))
		in.AptInstalled, in.AptCandidate = inst, cand
		in.UpgradeAvailable = cand != "" && inst != "" && cand != inst
	}

	return in, nil
}

func parseClamVersion(line string, in *Install) {
	m := versionRE.FindStringSubmatch(line)
	if m == nil {
		return
	}
	in.EngineVersion = m[1]
	in.DBVersion = m[2]
	in.DBDate = strings.TrimSpace(m[3])
}

// parseAptPolicy pulls the Installed/Candidate lines from `apt-cache policy`.
// "(none)" becomes "".
func parseAptPolicy(out string) (installed, candidate string) {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "Installed:"):
			installed = cleanAptVersion(strings.TrimPrefix(line, "Installed:"))
		case strings.HasPrefix(line, "Candidate:"):
			candidate = cleanAptVersion(strings.TrimPrefix(line, "Candidate:"))
		}
	}
	return installed, candidate
}

func cleanAptVersion(s string) string {
	s = strings.TrimSpace(s)
	if s == "(none)" {
		return ""
	}
	return s
}
