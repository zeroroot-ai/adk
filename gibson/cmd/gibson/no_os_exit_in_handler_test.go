// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package main

import (
	"path/filepath"
	"runtime"
	"testing"

	astchecks "github.com/zeroroot-ai/ast-checks"
)

// TestNoOsExitInHandler asserts that os.Exit() is never called outside of
// the CLI entry point (cmd/gibson/main.go). Deep os.Exit() calls in cobra
// command handlers or internal packages make the CLI code impossible to test
// and prevent callers from handling errors gracefully. They also bypass
// deferred cleanup (connection pool drains, temp-file removal, etc.).
//
// The correct pattern is to return an error from handler functions; the
// root cobra Execute() call in main.go receives the error and exits there.
//
// Scope: every .go file under the cli/ module root, excluding:
//   - _test.go files
//   - testdata/ directories (auto-skipped by the ast-checks harness)
//   - generated .pb.go / zz_generated*.go files
//
// Allowlist covers the canonical entry-point exit (main.go:22) and two
// pre-existing handler violations that need follow-up fixes.
//
// Implements slice 3.3 of the production-readiness epic
// (zeroroot-ai/.github#50).
func TestNoOsExitInHandler(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is at cmd/gibson/no_os_exit_in_handler_test.go.
	// The cli/ module root is three levels up (cmd/gibson/ → cmd/ → cli/).
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	matchers := []astchecks.Matcher{
		astchecks.NewForbiddenCallsite(
			"os.Exit() must not be called outside cmd/gibson/main.go — return errors instead",
			"os.Exit",
		),
	}

	// Existing-debt allowlist. Add entries here only for pre-existing
	// violations that cannot be fixed in this PR; every LEGACY-OPTIONAL
	// entry MUST carry a Reason with a tracked follow-up reference.
	//
	// Keys are CONTENT keys ("file :: snippet", astchecks.Finding.ContentKey),
	// never file:line. A header change or an added import moved the two
	// legitimate exits by eleven lines on 2026-09-05 and a line-keyed
	// allowlist silently stopped matching; content keys follow the code.
	allowlist := astchecks.Allowlist{
		// cmd/gibson/main.go: the one legitimate os.Exit site — translates
		// cobra's error return into a process exit code.
		"cmd/gibson/main.go :: os.Exit(exitErr.Code)": {
			Category: astchecks.CategoryDefensiveGuard,
			Reason:   "ExitCodeError exit site: propagates typed exit code from doRun/runValidate through cobra to the process exit",
		},
		"cmd/gibson/main.go :: os.Exit(1)": {
			Category: astchecks.CategoryDefensiveGuard,
			Reason:   "canonical entry-point exit: root.Execute() non-ExitCode error → os.Exit(1); this is the intended single exit site",
		},
	}

	opts := astchecks.WalkOpts{
		ScopeDirs: []string{moduleRoot},
		RepoRoot:  moduleRoot,
		Matchers:  matchers,
		Allowlist: allowlist,
		// Match allowlist entries on "file :: snippet", never on file:line.
		AllowlistByContent: true,
		SkipTestFiles:      true,
		SkipGenerated:      true,
	}

	findings, err := astchecks.Walk(opts)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(findings) > 0 {
		t.Errorf("os.Exit() calls found outside the CLI entry point (forbidden):\n%s\n\n"+
			"os.Exit() inside handler or internal code:\n"+
			"  - bypasses cobra's error handling\n"+
			"  - skips deferred cleanup (connection pool drain, temp files)\n"+
			"  - prevents unit-testing the handler without subprocess tricks\n\n"+
			"Fix: return an error from the handler. For exit-code propagation,\n"+
			"wrap the code in a custom error type (e.g. ExitCodeError) and\n"+
			"unwrap it in main() or root.Execute().\n\n"+
			"If this is an unavoidable pre-existing violation, add it to the\n"+
			"allowlist in this file with a CategoryLegacyOptional tag.\n",
			astchecks.RenderFindings(findings))
	}

	t.Logf("allowlist entries: %d", len(allowlist))
	if s := astchecks.FormatAllowlistLog(allowlist); s != "" {
		t.Log(s)
	}
}
