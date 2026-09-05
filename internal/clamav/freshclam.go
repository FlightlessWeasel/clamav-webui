package clamav

import (
	"context"
	"fmt"
	"path" // ClamAV DB paths are always POSIX on the Debian/Ubuntu target
	"strconv"
	"strings"
	"time"
)

// signatureDBs are the base names ClamAV keeps under the database directory,
// each as a .cvd (published) or .cld (incrementally patched) file.
var signatureDBs = []string{"main", "daily", "bytecode"}

// SignatureDB is metadata about one signature database file.
type SignatureDB struct {
	Name      string `json:"name"`       // main | daily | bytecode
	File      string `json:"file"`       // e.g. daily.cld ("" when absent)
	Present   bool   `json:"present"`    //
	Version   int    `json:"version"`    // 0 when unknown
	Sigs      int    `json:"sigs"`       // signature count, 0 when unknown
	BuildTime string `json:"build_time"` // as reported by sigtool
	MTimeUnix int64  `json:"mtime_unix"` // file modification time
}

// Signatures is the aggregate signature-freshness view.
type Signatures struct {
	DBDir            string        `json:"db_dir"`
	Databases        []SignatureDB `json:"databases"`
	TotalSigs        int           `json:"total_sigs"`
	NewestUnix       int64         `json:"newest_unix"`
	AgeSeconds       int64         `json:"age_seconds"` // now - NewestUnix, -1 if no DB present
	FreshclamService ServiceState  `json:"freshclam_service"`
	Checks           int           `json:"checks"` // freshclam.conf Checks/day, 0 if unset
	HasFreshclam     bool          `json:"has_freshclam"`
	HasSigtool       bool          `json:"has_sigtool"`
}

// Signatures reports the state of the signature databases and the updater.
func (m *Manager) Signatures(ctx context.Context) (Signatures, error) {
	sig := Signatures{DBDir: m.cfg.ClamAVDBDir}
	_, sig.HasFreshclam = m.run.LookPath("freshclam")
	_, sig.HasSigtool = m.run.LookPath("sigtool")

	for _, name := range signatureDBs {
		db := SignatureDB{Name: name}
		for _, ext := range []string{".cld", ".cvd"} {
			p := path.Join(m.cfg.ClamAVDBDir, name+ext)
			info, err := m.fsys.Stat(p)
			if err != nil {
				continue
			}
			db.Present = true
			db.File = name + ext
			db.MTimeUnix = info.ModTime().Unix()
			if sig.HasSigtool {
				if res, err := m.run.Run(ctx, Cmd{Name: "sigtool", Args: []string{"--info", p}}); err == nil {
					v, s, bt := parseSigtoolInfo(string(res.Stdout))
					db.Version, db.Sigs, db.BuildTime = v, s, bt
				}
			}
			break // .cld wins over .cvd
		}
		if db.Present {
			sig.TotalSigs += db.Sigs
			if db.MTimeUnix > sig.NewestUnix {
				sig.NewestUnix = db.MTimeUnix
			}
		}
		sig.Databases = append(sig.Databases, db)
	}

	if sig.NewestUnix == 0 {
		sig.AgeSeconds = -1
	} else {
		sig.AgeSeconds = time.Now().Unix() - sig.NewestUnix
	}

	svc, err := m.Service(ctx, "clamav-freshclam")
	if err != nil {
		return Signatures{}, err
	}
	sig.FreshclamService = svc

	if b, err := m.fsys.ReadFile(m.cfg.FreshclamConf); err == nil {
		if v := confValue(string(b), "Checks"); v != "" {
			sig.Checks, _ = strconv.Atoi(v)
		}
	}

	return sig, nil
}

// UpdateSignatures runs freshclam once, stopping the clamav-freshclam service
// first (it holds the DB lock) and restarting it afterwards if it was running.
func (m *Manager) UpdateSignatures(ctx context.Context, onLine func(string)) error {
	if onLine == nil {
		onLine = func(string) {}
	}
	if _, ok := m.run.LookPath("freshclam"); !ok {
		return ErrNotInstalled
	}

	svc, err := m.Service(ctx, "clamav-freshclam")
	wasActive := err == nil && svc.Active == "active"
	if wasActive {
		onLine("$ systemctl stop clamav-freshclam")
		if e := m.ServiceAction(ctx, "clamav-freshclam", "stop"); e != nil {
			onLine("warning: could not stop clamav-freshclam: " + e.Error())
		}
		defer func() {
			onLine("$ systemctl start clamav-freshclam")
			if e := m.ServiceAction(context.WithoutCancel(ctx), "clamav-freshclam", "start"); e != nil {
				onLine("warning: could not restart clamav-freshclam: " + e.Error())
			}
		}()
	}

	onLine("$ freshclam --stdout")
	if err := m.run.Stream(ctx, Cmd{Name: "freshclam", Args: []string{"--stdout"}}, onLine); err != nil {
		return fmt.Errorf("freshclam: %w", err)
	}
	return nil
}

// parseSigtoolInfo pulls Version / Signatures / Build time from `sigtool --info`.
func parseSigtoolInfo(out string) (version, sigs int, buildTime string) {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "Version":
			version, _ = strconv.Atoi(v)
		case "Signatures":
			sigs, _ = strconv.Atoi(v)
		case "Build time":
			buildTime = v
		}
	}
	return version, sigs, buildTime
}

// confValue returns the first value for key in a ClamAV-style config
// ("Key value" per line, # comments), or "".
func confValue(conf, key string) string {
	for _, raw := range strings.Split(conf, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			return strings.Join(fields[1:], " ")
		}
	}
	return ""
}
