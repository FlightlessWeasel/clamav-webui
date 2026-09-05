package clamav

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
)

func onaccMgr(t *testing.T, clamdConf string) (*Manager, *fakeRunner, *fakeFS) {
	t.Helper()
	fs := newFakeFS().put("clamd.conf", clamdConf, time.Now())
	f := newFake().have("clamscan", "clamdscan", "clamonacc", "freshclam")
	cfg := config.Defaults()
	cfg.ClamdConf = "/etc/clamav/clamd.conf"
	return NewManagerWithDeps(cfg, f, fs), f, fs
}

func TestOnAccessStatusReadsConf(t *testing.T) {
	m, f, _ := onaccMgr(t, "OnAccessIncludePath /home\nOnAccessPrevention yes\nOnAccessExcludeUname clamav\n")
	f.on(showKey("clamav-daemon"), fakeResp{stdout: "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"})
	f.on(showKey("clamav-clamonacc"), fakeResp{stdout: "Id=clamav-clamonacc.service\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nUnitFileState=disabled\n"})

	st, err := m.OnAccessStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Supported || !st.DaemonActive || st.Enabled {
		t.Fatalf("status = %+v", st)
	}
	if len(st.WatchPaths) != 1 || st.WatchPaths[0] != "/home" || !st.Prevention {
		t.Errorf("watch/prevention = %v / %v", st.WatchPaths, st.Prevention)
	}
}

func TestApplyOnAccessEnable(t *testing.T) {
	m, f, fs := onaccMgr(t, "LogVerbose no\n")
	f.on(showKey("clamav-daemon"), fakeResp{stdout: "Id=clamav-daemon.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"})
	f.on("systemctl enable clamav-daemon", fakeResp{})
	f.on("systemctl restart clamav-daemon", fakeResp{})
	f.on("systemctl enable clamav-clamonacc", fakeResp{})
	f.on("systemctl restart clamav-clamonacc", fakeResp{})

	err := m.ApplyOnAccess(context.Background(), OnAccessConfig{
		Enabled: true, Paths: []string{"/srv", "/home"}, Prevention: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	conf, _ := fs.ReadFile("clamd.conf")
	body := string(conf)
	if strings.Count(body, "OnAccessIncludePath ") != 2 {
		t.Errorf("include paths not written:\n%s", body)
	}
	if !strings.Contains(body, "OnAccessPrevention yes") || !strings.Contains(body, "OnAccessExcludeUname clamav") {
		t.Errorf("prevention/uname not written:\n%s", body)
	}
	seq := strings.Join(f.calls, " | ")
	if !strings.Contains(seq, "systemctl restart clamav-daemon") ||
		!strings.Contains(seq, "systemctl enable clamav-clamonacc") ||
		!strings.Contains(seq, "systemctl restart clamav-clamonacc") {
		t.Errorf("service sequence wrong: %s", seq)
	}
}

func TestApplyOnAccessDisable(t *testing.T) {
	m, f, _ := onaccMgr(t, "OnAccessIncludePath /home\n")
	f.on("systemctl stop clamav-clamonacc", fakeResp{})
	f.on("systemctl disable clamav-clamonacc", fakeResp{})

	if err := m.ApplyOnAccess(context.Background(), OnAccessConfig{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	seq := strings.Join(f.calls, " | ")
	if !strings.Contains(seq, "systemctl disable clamav-clamonacc") {
		t.Errorf("disable not called: %s", seq)
	}
	if strings.Contains(seq, "systemctl restart clamav-daemon") {
		t.Error("daemon should not be restarted on disable")
	}
}

func TestApplyOnAccessRequiresClamonacc(t *testing.T) {
	fs := newFakeFS().put("clamd.conf", "", time.Now())
	m := NewManagerWithDeps(config.Defaults(), newFake(), fs) // clamonacc not present
	if err := m.ApplyOnAccess(context.Background(), OnAccessConfig{Enabled: true}); err != ErrNotInstalled {
		t.Fatalf("err = %v", err)
	}
}
