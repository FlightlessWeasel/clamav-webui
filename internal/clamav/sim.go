package clamav

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SimRunner is an in-memory fake of the ClamAV/apt/systemd toolchain for
// developing the UI on a machine without ClamAV (e.g. Windows). Enable it by
// setting CLAMWEB_DEV_SIM=1. It is intentionally not wired into production
// unless that variable is present.
type SimRunner struct {
	mu        sync.Mutex
	installed bool
	engine    string
	dbVersion string
	dbDate    string
	units     map[string]*simUnit
}

type simUnit struct {
	active, enabled bool
	since           time.Time
}

// NewSimRunner returns a SimRunner seeded as "ClamAV not installed".
func NewSimRunner() *SimRunner {
	return &SimRunner{
		engine:    "1.4.1",
		dbVersion: "27342",
		dbDate:    time.Now().Add(-26 * time.Hour).Format("Mon Jan 2 15:04:05 2006"),
		units: map[string]*simUnit{
			"clamav-daemon":    {},
			"clamav-freshclam": {},
			"clamav-clamonacc": {},
		},
	}
}

func (s *SimRunner) LookPath(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "clamscan", "clamdscan", "freshclam", "clamonacc", "sigtool":
		return "/usr/bin/" + name, s.installed
	case "systemctl", "journalctl", "apt-get", "apt-cache":
		return "/usr/bin/" + name, true
	}
	return "", false
}

func (s *SimRunner) Run(_ context.Context, c Cmd) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	args := strings.Join(c.Args, " ")

	switch c.Name {
	case "clamscan":
		if args == "--version" {
			if s.dbVersion != "" {
				return out("ClamAV %s/%s/%s\n", s.engine, s.dbVersion, s.dbDate), nil
			}
			return out("ClamAV %s\n", s.engine), nil
		}
	case "apt-cache":
		if strings.HasPrefix(args, "policy clamav") {
			inst := "(none)"
			if s.installed {
				inst = "1.4.1+dfsg-1"
			}
			return out("clamav:\n  Installed: %s\n  Candidate: 1.4.1+dfsg-1\n", inst), nil
		}
	case "systemctl":
		return s.systemctl(c.Args)
	case "journalctl":
		return out("-- sim: no journal --\n"), nil
	}
	return Result{}, fmt.Errorf("sim: unhandled command %q %q", c.Name, args)
}

func (s *SimRunner) systemctl(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{}, fmt.Errorf("sim: systemctl with no args")
	}
	verb := args[0]
	if verb == "show" {
		var b strings.Builder
		wrote := false
		for _, name := range args[1:] {
			if strings.HasPrefix(name, "--") {
				continue
			}
			if wrote {
				b.WriteString("\n")
			}
			wrote = true
			u := s.units[name]
			if u == nil {
				fmt.Fprintf(&b, "Id=%s.service\nLoadState=not-found\nActiveState=inactive\nSubState=dead\nUnitFileState=\n", name)
				continue
			}
			active, sub, enabled := "inactive", "dead", "disabled"
			if u.active {
				active, sub = "active", "running"
			}
			if u.enabled {
				enabled = "enabled"
			}
			ts := ""
			if !u.since.IsZero() {
				ts = u.since.Format("Mon 2006-01-02 15:04:05 MST")
			}
			fmt.Fprintf(&b, "Id=%s.service\nLoadState=loaded\nActiveState=%s\nSubState=%s\nUnitFileState=%s\nActiveEnterTimestamp=%s\n",
				name, active, sub, enabled, ts)
		}
		return Result{Stdout: []byte(b.String())}, nil
	}

	if len(args) >= 2 {
		u := s.units[args[1]]
		if u == nil {
			return Result{ExitCode: 5}, fmt.Errorf("sim: unknown unit %s", args[1])
		}
		switch verb {
		case "start", "restart":
			u.active = true
			u.since = time.Now()
		case "stop":
			u.active = false
		case "enable":
			u.enabled = true
		case "disable":
			u.enabled = false
		}
		return Result{}, nil
	}
	return Result{}, nil
}

func (s *SimRunner) Stream(_ context.Context, c Cmd, onLine func(string)) error {
	args := strings.Join(c.Args, " ")

	if c.Name == "apt-get" && strings.HasPrefix(args, "update") {
		for _, l := range []string{"Hit:1 http://deb.debian.org/debian bookworm InRelease", "Reading package lists... Done"} {
			onLine(l)
			time.Sleep(120 * time.Millisecond)
		}
		return nil
	}
	if c.Name == "apt-get" && strings.HasPrefix(args, "install") {
		for _, l := range []string{
			"Reading package lists... Done",
			"Building dependency tree... Done",
			"The following NEW packages will be installed:",
			"  clamav clamav-daemon clamav-freshclam",
			"Setting up clamav-freshclam ...",
			"Setting up clamav-daemon ...",
			"Processing triggers for systemd ...",
		} {
			onLine(l)
			time.Sleep(200 * time.Millisecond)
		}
		s.mu.Lock()
		s.installed = true
		s.units["clamav-freshclam"].active = true
		s.units["clamav-freshclam"].enabled = true
		s.units["clamav-daemon"].active = true
		s.units["clamav-daemon"].enabled = true
		now := time.Now()
		s.units["clamav-freshclam"].since = now
		s.units["clamav-daemon"].since = now
		s.mu.Unlock()
		return nil
	}
	if c.Name == "freshclam" {
		for _, l := range []string{
			"ClamAV update process started at " + time.Now().Format(time.RFC1123),
			"daily.cld updated (version: 27343, sigs: 2069419)",
			"bytecode.cld is up-to-date",
		} {
			onLine(l)
			time.Sleep(200 * time.Millisecond)
		}
		s.mu.Lock()
		s.dbVersion = "27343"
		s.dbDate = time.Now().Format("Mon Jan 2 15:04:05 2006")
		s.mu.Unlock()
		return nil
	}
	return fmt.Errorf("sim: unhandled stream command %q %q", c.Name, args)
}

func out(format string, a ...any) Result {
	return Result{Stdout: []byte(fmt.Sprintf(format, a...))}
}
