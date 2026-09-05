package clamav

import (
	"context"
	"fmt"
	"strings"
)

// fakeRunner is a scripted Runner for tests. Keys are "name arg1 arg2 ...".
type fakeRunner struct {
	responses map[string]fakeResp
	present   map[string]bool // executables on PATH
	calls     []string
}

type fakeResp struct {
	stdout string
	stderr string
	err    error
	lines  []string // for Stream
}

func newFake() *fakeRunner {
	return &fakeRunner{
		responses: map[string]fakeResp{},
		present:   map[string]bool{},
	}
}

func key(c Cmd) string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

func (f *fakeRunner) on(cmdline string, r fakeResp) *fakeRunner {
	f.responses[cmdline] = r
	return f
}

func (f *fakeRunner) have(names ...string) *fakeRunner {
	for _, n := range names {
		f.present[n] = true
	}
	return f
}

func (f *fakeRunner) Run(_ context.Context, c Cmd) (Result, error) {
	k := key(c)
	f.calls = append(f.calls, k)
	r, ok := f.responses[k]
	if !ok {
		return Result{ExitCode: 127}, fmt.Errorf("fakeRunner: no response scripted for %q", k)
	}
	res := Result{Stdout: []byte(r.stdout), Stderr: []byte(r.stderr)}
	if r.err != nil {
		res.ExitCode = 1
	}
	return res, r.err
}

func (f *fakeRunner) Stream(_ context.Context, c Cmd, onLine func(string)) error {
	k := key(c)
	f.calls = append(f.calls, k)
	r, ok := f.responses[k]
	if !ok {
		return fmt.Errorf("fakeRunner: no response scripted for %q", k)
	}
	for _, ln := range r.lines {
		onLine(ln)
	}
	if r.stdout != "" {
		for _, ln := range strings.Split(strings.TrimRight(r.stdout, "\n"), "\n") {
			onLine(ln)
		}
	}
	return r.err
}

func (f *fakeRunner) LookPath(name string) (string, bool) {
	if f.present[name] {
		return "/usr/bin/" + name, true
	}
	return "", false
}
