// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package runner is a small process supervisor for `gibson component run`.
//
// It exec's the component's compiled binary, forwards stdout/stderr,
// hooks SIGINT/SIGTERM and propagates them to the child, waits up to
// DrainTimeout for graceful shutdown before SIGKILL, and surfaces the
// child's exit code (notably treating exit 75 as the SDK's plugin
// rotation contract — the parent does not interpret 75 as failure).
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// ExitCodeRotation is the conventional exit code the SDK's plugin
// runtime uses to signal "secret rotated; restart me". Documented in
// AGENTS.md (plugin kind).
const ExitCodeRotation = 75

// RunOptions configures a single supervised run.
type RunOptions struct {
	// Binary is the path to the compiled component binary. If relative,
	// it is resolved against the caller's working directory.
	Binary string

	// Args are forwarded to the binary unchanged.
	Args []string

	// Env is additive to os.Environ(). Each entry is "KEY=VALUE".
	Env []string

	// DrainTimeout is the maximum time runner waits between sending
	// SIGTERM and sending SIGKILL when the parent is signalled. A zero
	// value uses 30s.
	DrainTimeout time.Duration

	// Stdout / Stderr are where the child's streams go. nil values use
	// os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// Run starts the binary, supervises it, and returns its exit code.
//
// The returned error is nil on a clean child exit (regardless of the
// child's exit code — exit 1 from the child is normal-from-runner's-
// perspective). It is non-nil only for setup failures (binary not
// found, etc.) and for genuinely unexpected supervisor errors.
//
// Cancelling ctx triggers the same drain logic as receiving SIGTERM.
func Run(ctx context.Context, opts RunOptions) (exitCode int, err error) {
	if opts.Binary == "" {
		return 0, errors.New("runner: Binary is required")
	}
	if opts.DrainTimeout == 0 {
		opts.DrainTimeout = 30 * time.Second
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}

	// LookPath: gives a clear error before we fork.
	if _, err := exec.LookPath(opts.Binary); err != nil {
		return 0, fmt.Errorf("runner: %s not found (did you `make build`?): %w", opts.Binary, err)
	}

	cmd := exec.Command(opts.Binary, opts.Args...) //nolint:gosec // intended exec
	cmd.Env = append(os.Environ(), opts.Env...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("runner: start %s: %w", opts.Binary, err)
	}

	// Forward SIGINT/SIGTERM (plus the parent context's cancellation)
	// to the child. On signal, send SIGTERM first; if the child hasn't
	// exited within DrainTimeout, escalate to SIGKILL.
	signalCtx, signalStop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer signalStop()

	supervisorErr := make(chan error, 1)
	go func() {
		select {
		case <-signalCtx.Done():
			// Best effort: forward SIGTERM to the child.
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
			// Wait up to DrainTimeout for graceful exit, then SIGKILL.
			timer := time.NewTimer(opts.DrainTimeout)
			defer timer.Stop()
			select {
			case <-supervisorErr:
				// Wait() returned before timeout; nothing more to do.
				return
			case <-timer.C:
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}
		case <-supervisorErr:
			// Child exited on its own; nothing to do.
		}
	}()

	waitErr := cmd.Wait()
	supervisorErr <- waitErr

	// exec.ExitError carries the real exit code; nil means clean exit 0.
	if waitErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, fmt.Errorf("runner: wait %s: %w", opts.Binary, waitErr)
}
