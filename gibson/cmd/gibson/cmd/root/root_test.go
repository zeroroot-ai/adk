// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package root

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRootCommandSurface pins the set of top-level `gibson` commands.
// Adding or removing a top-level command — including a back-compat
// shim like `gibson plugin` / `gibson agent` / `gibson tool` — fails
// this test, forcing the change to be visible at PR review.
//
// Subcommands and flags are intentionally NOT covered; only the
// outer verb surface is the load-bearing contract.
func TestRootCommandSurface(t *testing.T) {
	want := []string{"agent", "agent-skill", "component", "connector", "docs", "init", "inspect", "login", "logout", "mission", "secret", "target"}

	got := make([]string, 0, len(rootCmd.Commands()))
	for _, c := range rootCmd.Commands() {
		got = append(got, c.Name())
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Fatalf("root command surface drifted:\n  want %v\n  got  %v", want, got)
	}
}

// missingFromHelp returns each subcommand of cmd that has no line of its own
// in the Subcommands list of the Long text. A line starts with two spaces, the
// command name and one or more spaces.
func missingFromHelp(cmd *cobra.Command) []string {
	listed := map[string]bool{}
	for _, line := range strings.Split(cmd.Long, "\n") {
		if f := strings.Fields(line); strings.HasPrefix(line, "  ") && len(f) > 1 {
			listed[f[0]] = true
		}
	}
	var missing []string
	for _, c := range cmd.Commands() {
		if !listed[c.Name()] {
			missing = append(missing, c.Name())
		}
	}
	return missing
}

// TestRootHelpListsEachCommand fails when a top-level command has no line in
// the root help text, as `secret` had none until this test.
func TestRootHelpListsEachCommand(t *testing.T) {
	if missing := missingFromHelp(rootCmd); len(missing) > 0 {
		t.Fatalf("the root help text lists no line for: %v", missing)
	}
}

// TestMissingFromHelpFindsAnUnlistedCommand is the failing fixture of
// TestRootHelpListsEachCommand.
func TestMissingFromHelpFindsAnUnlistedCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Long: "Subcommands:\n  listed  a command\n  other   the word secret in a description"}
	cmd.AddCommand(&cobra.Command{Use: "listed"}, &cobra.Command{Use: "secret"})
	if got := missingFromHelp(cmd); !slices.Equal(got, []string{"secret"}) {
		t.Fatalf("missingFromHelp = %v, want [secret]", got)
	}
}
