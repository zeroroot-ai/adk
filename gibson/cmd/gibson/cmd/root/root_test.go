// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package root

import (
	"slices"
	"testing"
)

// TestRootCommandSurface pins the set of top-level `gibson` commands.
// Adding or removing a top-level command — including a back-compat
// shim like `gibson plugin` / `gibson agent` / `gibson tool` — fails
// this test, forcing the change to be visible at PR review.
//
// Subcommands and flags are intentionally NOT covered; only the
// outer verb surface is the load-bearing contract.
func TestRootCommandSurface(t *testing.T) {
	want := []string{"agent", "component", "connector", "docs", "init", "inspect", "login", "logout", "mission", "target"}

	got := make([]string, 0, len(rootCmd.Commands()))
	for _, c := range rootCmd.Commands() {
		got = append(got, c.Name())
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Fatalf("root command surface drifted:\n  want %v\n  got  %v", want, got)
	}
}
