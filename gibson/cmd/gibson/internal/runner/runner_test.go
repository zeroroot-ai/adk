// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package runner_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/runner"
)

// fixturePath is the compiled runner_fixture binary, built once per
// test process by TestMain into a process-level tempdir that survives
// across t.TempDir cleanup.
var fixturePath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "runner-fixture-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "build runner fixture: mkdir: %v\n", err)
		os.Exit(2)
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "runner_fixture")

	cmd := exec.Command("go", "build", "-o", bin, "./testdata/runner_fixture")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build runner fixture failed:\n%s\n%v\n", out, err)
		os.Exit(2)
	}
	fixturePath = bin

	os.Exit(m.Run())
}

func buildFixture(t *testing.T) string {
	t.Helper()
	require.NotEmpty(t, fixturePath, "fixture binary not built; TestMain misconfigured")
	return fixturePath
}

func TestRun_ExitZero(t *testing.T) {
	bin := buildFixture(t)
	var stdout bytes.Buffer
	code, err := runner.Run(context.Background(), runner.RunOptions{
		Binary: bin,
		Args:   []string{"exit-zero"},
		Stdout: &stdout,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "ready")
}

func TestRun_NonZeroExit(t *testing.T) {
	bin := buildFixture(t)
	code, err := runner.Run(context.Background(), runner.RunOptions{
		Binary: bin,
		Args:   []string{"exit-1"},
	})
	require.NoError(t, err) // child non-zero is not a runner error
	assert.Equal(t, 1, code)
}

func TestRun_Exit75Rotation(t *testing.T) {
	bin := buildFixture(t)
	code, err := runner.Run(context.Background(), runner.RunOptions{
		Binary: bin,
		Args:   []string{"exit-75"},
	})
	require.NoError(t, err)
	assert.Equal(t, runner.ExitCodeRotation, code)
	assert.Equal(t, 75, code)
}

func TestRun_DrainTimeoutEscalatesToKill(t *testing.T) {
	if testing.Short() {
		t.Skip("drain-timeout test is slow; skipped under -short")
	}
	bin := buildFixture(t)

	// Cancel ctx after the child has had time to install its SIGTERM
	// handler; runner should send SIGTERM, wait DrainTimeout=300ms,
	// then SIGKILL.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	code, err := runner.Run(ctx, runner.RunOptions{
		Binary:       bin,
		Args:         []string{"ignore-sigterm"},
		DrainTimeout: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	// SIGKILL means the process was killed by signal 9.
	// runner returns the ExitError's exit code; on signal it's -1.
	assert.True(t, code == -1 || code == 128+int(syscall.SIGKILL),
		"expected kill exit code, got %d", code)
	assert.Less(t, elapsed, 5*time.Second, "drain-timeout escalation took too long")
}

func TestRun_BinaryNotFound(t *testing.T) {
	_, err := runner.Run(context.Background(), runner.RunOptions{
		Binary: "/nonexistent/binary",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRun_RequiresBinary(t *testing.T) {
	_, err := runner.Run(context.Background(), runner.RunOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Binary is required")
}
