package clamav

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fakeFS is an in-memory clamav.FS. Keys are matched by basename.
type fakeFS struct {
	files map[string]fakeFile
}

type fakeFile struct {
	content []byte
	mod     time.Time
}

func newFakeFS() *fakeFS { return &fakeFS{files: map[string]fakeFile{}} }

func (f *fakeFS) put(name string, content string, mod time.Time) *fakeFS {
	f.files[name] = fakeFile{content: []byte(content), mod: mod}
	return f
}

func (f *fakeFS) Stat(name string) (os.FileInfo, error) {
	if ff, ok := f.files[filepath.Base(name)]; ok {
		return fakeInfo{name: filepath.Base(name), size: int64(len(ff.content)), mod: ff.mod}, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeFS) ReadFile(name string) ([]byte, error) {
	if ff, ok := f.files[filepath.Base(name)]; ok {
		return ff.content, nil
	}
	return nil, os.ErrNotExist
}

type fakeInfo struct {
	name string
	size int64
	mod  time.Time
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return i.size }
func (i fakeInfo) Mode() os.FileMode  { return 0o644 }
func (i fakeInfo) ModTime() time.Time { return i.mod }
func (i fakeInfo) IsDir() bool        { return false }
func (i fakeInfo) Sys() any           { return nil }

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
	exit   int      // non-zero => Stream returns *ExitError{Code: exit}
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
	if r.exit != 0 {
		return &ExitError{Code: r.exit}
	}
	return r.err
}

func (f *fakeRunner) LookPath(name string) (string, bool) {
	if f.present[name] {
		return "/usr/bin/" + name, true
	}
	return "", false
}
