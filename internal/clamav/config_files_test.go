package clamav

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

const sampleClamd = `# clamd.conf
LogFile /var/log/clamav/clamav.log
LogVerbose false
MaxThreads 12
OnAccessIncludePath /home
OnAccessIncludePath /srv
# a comment
User clamav
`

func confMgr(t *testing.T, files map[string]string) (*Manager, *fakeFS) {
	t.Helper()
	fs := newFakeFS()
	for name, body := range files {
		fs.put(name, body, time.Now())
	}
	cfg := config.Defaults()
	cfg.ClamdConf = "/etc/clamav/clamd.conf"
	cfg.FreshclamConf = "/etc/clamav/freshclam.conf"
	return NewManagerWithDeps(cfg, newFake(), fs), fs
}

func TestReadConfWhitelistOnly(t *testing.T) {
	m, _ := confMgr(t, map[string]string{"clamd.conf": sampleClamd})
	v, err := m.ReadConf("clamd")
	if err != nil {
		t.Fatal(err)
	}

	got := map[string][]string{}
	for _, e := range v.Entries {
		got[e.Name] = e.Values
	}
	if len(got["OnAccessIncludePath"]) != 2 || got["OnAccessIncludePath"][0] != "/home" {
		t.Errorf("OnAccessIncludePath = %v", got["OnAccessIncludePath"])
	}
	if len(got["MaxThreads"]) != 1 || got["MaxThreads"][0] != "12" {
		t.Errorf("MaxThreads = %v", got["MaxThreads"])
	}
	// Non-whitelisted keys never appear as entries.
	if _, ok := got["User"]; ok {
		t.Error("User should not be exposed as an editable entry")
	}
	if _, ok := got["LogFile"]; ok {
		t.Error("LogFile should not be exposed")
	}
}

func TestWriteConfPreservesUnknownLinesAndKeepsBak(t *testing.T) {
	m, fs := confMgr(t, map[string]string{"clamd.conf": sampleClamd})

	err := m.WriteConf("clamd", map[string][]string{
		"MaxThreads":          {"8"},
		"OnAccessIncludePath": {"/data", "/mnt/media"},
		"LogVerbose":          {"yes"},
	})
	if err != nil {
		t.Fatal(err)
	}

	out, _ := fs.ReadFile("clamd.conf")
	body := string(out)
	// Untouched lines survive.
	for _, want := range []string{"LogFile /var/log/clamav/clamav.log", "User clamav", "# a comment"} {
		if !strings.Contains(body, want) {
			t.Errorf("write dropped %q\n---\n%s", want, body)
		}
	}
	// Old values gone, new values present, exactly once.
	if strings.Contains(body, "MaxThreads 12") || !strings.Contains(body, "MaxThreads 8") {
		t.Errorf("MaxThreads not replaced:\n%s", body)
	}
	if strings.Count(body, "OnAccessIncludePath ") != 2 {
		t.Errorf("expected 2 OnAccessIncludePath lines:\n%s", body)
	}
	if !strings.Contains(body, "OnAccessIncludePath /data") || !strings.Contains(body, "OnAccessIncludePath /mnt/media") {
		t.Errorf("new include paths missing:\n%s", body)
	}
	if !strings.Contains(body, "LogVerbose yes") {
		t.Errorf("LogVerbose not added:\n%s", body)
	}

	bak, err := fs.ReadFile("clamd.conf.bak")
	if err != nil || string(bak) != sampleClamd {
		t.Errorf("backup not written with original content")
	}
}

func TestWriteConfRemovesKeyOnEmptyValue(t *testing.T) {
	m, fs := confMgr(t, map[string]string{"clamd.conf": sampleClamd})
	if err := m.WriteConf("clamd", map[string][]string{"OnAccessIncludePath": {}}); err != nil {
		t.Fatal(err)
	}
	out, _ := fs.ReadFile("clamd.conf")
	if strings.Contains(string(out), "OnAccessIncludePath") {
		t.Errorf("key not removed:\n%s", out)
	}
}

func TestWriteConfRejectsNonWhitelistedKey(t *testing.T) {
	m, _ := confMgr(t, map[string]string{"clamd.conf": sampleClamd})
	err := m.WriteConf("clamd", map[string][]string{"User": {"root"}})
	if !errors.Is(err, ErrConfKeyNotAllowed) {
		t.Fatalf("err = %v, want ErrConfKeyNotAllowed", err)
	}
}

func TestReadConfUnknownTarget(t *testing.T) {
	m, _ := confMgr(t, nil)
	if _, err := m.ReadConf("sshd"); !errors.Is(err, ErrUnknownConf) {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteConfValidatesValues(t *testing.T) {
	m, fs := confMgr(t, map[string]string{"clamd.conf": sampleClamd})
	bad := []map[string][]string{
		{"MaxThreads": {"lots"}},
		{"MaxFileSize": {"huge"}},
		{"OnAccessIncludePath": {"relative/path"}},
	}
	for _, u := range bad {
		if err := m.WriteConf("clamd", u); !errors.Is(err, ErrConfValueInvalid) {
			t.Errorf("WriteConf(%v) err = %v, want ErrConfValueInvalid", u, err)
		}
	}
	// The file was not touched by a rejected write.
	out, _ := fs.ReadFile("clamd.conf")
	if string(out) != sampleClamd {
		t.Errorf("file changed by a rejected write:\n%s", out)
	}
}

func TestWriteConfInPlaceAndBoolNormalise(t *testing.T) {
	m, fs := confMgr(t, map[string]string{
		"clamd.conf": "LogVerbose false\nMaxThreads 12\n# trailing comment\nUser clamav\n",
	})
	if err := m.WriteConf("clamd", map[string][]string{"LogVerbose": {"1"}}); err != nil {
		t.Fatal(err)
	}
	out, _ := fs.ReadFile("clamd.conf")
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	// LogVerbose stays on line 1 (in place), normalised to "yes".
	if lines[0] != "LogVerbose yes" {
		t.Errorf("line 0 = %q, want 'LogVerbose yes'", lines[0])
	}
	if lines[len(lines)-1] != "User clamav" {
		t.Errorf("trailing lines disturbed:\n%s", out)
	}
}
