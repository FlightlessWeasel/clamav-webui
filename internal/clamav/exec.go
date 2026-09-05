// Package clamav is the domain layer between the HTTP API and the ClamAV
// toolchain (clamscan, clamdscan, freshclam, sigtool) plus the apt and systemd
// commands used to install and run it. Every external call goes through Runner
// so the package is testable with a fake.
package clamav

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// maxCapturedOutput bounds how much stdout/stderr Run keeps in memory. ClamAV
// and apt output is small; anything larger is almost certainly a runaway.
const maxCapturedOutput = 8 << 20 // 8 MiB

// ExitError reports that a streamed command exited non-zero. Callers that care
// about the specific code (clamscan uses 1 for "virus found") can errors.As it.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return "command exited with status " + strconv.Itoa(e.Code)
}

// Cmd describes one external command invocation.
type Cmd struct {
	Name string
	Args []string
	// Env entries (KEY=VALUE) are appended to the current environment.
	Env []string
}

// Result is the outcome of a completed Run.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner executes external commands.
type Runner interface {
	// Run executes c to completion and returns its output. A non-zero exit is
	// reported both in Result.ExitCode and as a non-nil error.
	Run(ctx context.Context, c Cmd) (Result, error)
	// Stream executes c, invoking onLine for every line written to stdout or
	// stderr, in arrival order. It returns the command's exit error, if any.
	Stream(ctx context.Context, c Cmd, onLine func(string)) error
	// LookPath resolves an executable name against PATH.
	LookPath(name string) (string, bool)
}

// execRunner is the production Runner backed by os/exec.
type execRunner struct{}

// NewExecRunner returns a Runner that shells out for real.
func NewExecRunner() Runner { return execRunner{} }

func (execRunner) Run(ctx context.Context, c Cmd) (Result, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, n: maxCapturedOutput}
	cmd.Stderr = &limitedWriter{buf: &stderr, n: maxCapturedOutput}

	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	}
	return res, err
}

func (execRunner) Stream(ctx context.Context, c Cmd, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return err
	}

	// The reader goroutine must consume pr to EOF no matter what, or cmd.Wait's
	// internal stdout/stderr copy will block writing to pw and never return.
	// A manual ReadString loop (unlike bufio.Scanner) tolerates arbitrarily
	// long lines instead of stopping early.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		br := bufio.NewReaderSize(pr, 64*1024)
		for {
			line, err := br.ReadString('\n')
			if len(line) > 0 {
				onLine(strings.TrimRight(line, "\r\n"))
			}
			if err != nil {
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	pw.Close() // unblock the reader with EOF
	wg.Wait()
	pr.Close()

	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		return &ExitError{Code: ee.ExitCode()}
	}
	return waitErr
}

func (execRunner) LookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	return p, err == nil
}

// limitedWriter copies into buf until n bytes have been written, then silently
// discards the rest so a runaway command can't exhaust memory.
type limitedWriter struct {
	buf *bytes.Buffer
	n   int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.n > 0 {
		take := len(p)
		if take > w.n {
			take = w.n
		}
		w.buf.Write(p[:take])
		w.n -= take
	}
	return len(p), nil
}
