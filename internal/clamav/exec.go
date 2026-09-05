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
	"sync"
)

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
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			onLine(sc.Text())
		}
	}()

	waitErr := cmd.Wait()
	pw.Close()
	wg.Wait()
	pr.Close()
	return waitErr
}

func (execRunner) LookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	return p, err == nil
}
