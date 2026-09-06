// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

// check_cue_fresh_test exercises the CUE freshness gate
// (../../../../../scripts/check-cue-fresh.sh) against a deliberately-stale
// fixture and asserts the script exits non-zero with a clear "STALE"
// message that surfaces the dropped field.
//
// This is the negative half of the freshness contract. The positive
// half — that the committed embedded CUE is fresh — runs as `make
// check-cue-fresh` in CI, not as a Go test (the regen requires the SDK
// sibling clone and a `cue` binary, neither of which is in the default
// `go test` environment).
//
// The negative test runs everywhere because we only need the script to
// be executable and to detect the missing-sentinel-or-malformed-file
// failure modes, which it does without invoking cue at all.
//
// Spec: zeroroot-ai/adk#27 (CUE freshness gate, M3 of mission-author-
// experience epic, parent PRD zeroroot-ai/gibson#131).

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file to find the adk repository root.
// We need an absolute path because the freshness check script and the
// stale fixture live at predictable repo-relative paths.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../adk/cli/cmd/gibson/cmd/mission/check_cue_fresh_test.go
	// filepath.Dir starts at .../mission/; walk up five more dirs
	// (cmd, gibson, cmd, cli, adk) to reach the adk repo root.
	dir := filepath.Dir(thisFile)
	for range 5 {
		dir = filepath.Dir(dir)
	}
	return dir
}

// TestCheckCueFresh_DetectsStaleFixture is the required negative test
// from zeroroot-ai/adk#27 acceptance criteria: feed the freshness
// checker a fixture with a known intentional drift and assert
// non-zero exit + an actionable error message.
//
// The fixture (scripts/testdata/stale_mission_definition.cue) is a
// snapshot of the embedded CUE with MissionDefinition.workspace
// surgically removed. When the SDK sibling is present (workstation,
// polyrepo CI), the check should:
//   - exit with status 1 (STALE, not a script bug),
//   - print "STALE: run 'make generate' to refresh embedded CUE",
//   - print a unified diff naming the missing `workspace` field.
//
// When the SDK sibling is absent (ADK-only CI), the check falls back
// to STRUCTURAL mode, which inspects the sentinel header only and
// would pass the fixture (the fixture preserves the sentinel by
// design). The test skips in that environment with a clear message.
func TestCheckCueFresh_DetectsStaleFixture(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "check-cue-fresh.sh")
	fixture := filepath.Join(root, "scripts", "testdata", "stale_mission_definition.cue")
	sdkProto := filepath.Join(root, "..", "sdk", "api", "proto", "gibson", "mission", "v1")

	if _, err := os.Stat(script); err != nil {
		t.Fatalf("freshness script missing at %s: %v", script, err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("stale fixture missing at %s: %v", fixture, err)
	}
	if _, err := os.Stat(sdkProto); err != nil {
		t.Skipf("SDK sibling not present at %s — STRUCTURAL mode would pass; skipping FULL-mode negative test", sdkProto)
	}
	if _, err := exec.LookPath("cue"); err != nil {
		t.Skip("cue binary not on PATH — install with `go install cuelang.org/go/cmd/cue@v0.16.1` to run this test")
	}

	cmd := exec.Command(script)
	cmd.Env = append(os.Environ(), "ADK_CUE_FIXTURE="+fixture)
	out, err := cmd.CombinedOutput()
	combined := string(out)

	// The script must exit non-zero on the staleness path.
	exitErr := &exec.ExitError{}
	ok := errors.As(err, &exitErr)
	if err == nil {
		t.Fatalf("expected check-cue-fresh.sh to exit non-zero against stale fixture, got success.\nOutput:\n%s", combined)
	}
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v\nOutput:\n%s", err, err, combined)
	}
	if got := exitErr.ExitCode(); got != 1 {
		t.Errorf("expected exit code 1 (STALE), got %d\nOutput:\n%s", got, combined)
	}

	// The error message must point the human at the regen target.
	const wantInstruction = "STALE: run 'make generate' to refresh embedded CUE"
	if !strings.Contains(combined, wantInstruction) {
		t.Errorf("expected output to contain %q, got:\n%s", wantInstruction, combined)
	}

	// The diff must surface the dropped field name so the reviewer
	// knows WHAT drifted, not just THAT something did.
	const wantField = "workspace?: #WorkspaceConfig"
	if !strings.Contains(combined, wantField) {
		t.Errorf("expected diff to mention dropped field %q, got:\n%s", wantField, combined)
	}
}
