// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package docs

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

// syntheticTree builds a small command tree exercising the exporter's edges:
// a persistent (global) flag, a local flag, a boolean flag whose false default
// is elided, a nested subcommand, and the auto-generated help/completion
// commands that must be excluded.
func syntheticTree() *cobra.Command {
	noop := func(*cobra.Command, []string) error { return nil }
	root := &cobra.Command{Use: "gibson", Short: "root"}
	root.PersistentFlags().String("ca-cert", "", "trust extra CA")

	parent := &cobra.Command{Use: "component", Short: "components"}
	child := &cobra.Command{Use: "build [flags]", Short: "build it", Long: "build long\n", RunE: noop}
	child.Flags().Bool("force", false, "overwrite")
	child.Flags().String("output", "out", "output dir")
	parent.AddCommand(child)

	// Added out of alpha order to prove the exporter sorts.
	root.AddCommand(parent)
	root.AddCommand(&cobra.Command{Use: "agent", Short: "agents", RunE: noop})
	// cobra normally injects these; add explicitly (runnable, so the only thing
	// excluding them is the name filter) to prove they are dropped.
	root.AddCommand(&cobra.Command{Use: "completion", Short: "shell completion", RunE: noop})
	root.SetHelpCommand(&cobra.Command{Use: "help", Short: "help", RunE: noop})
	return root
}

func TestBuildCLISpec_ShapeAndOrder(t *testing.T) {
	spec := BuildCLISpec(syntheticTree())

	if spec.Binary != "gibson" {
		t.Fatalf("binary = %q, want gibson", spec.Binary)
	}
	if len(spec.GlobalFlags) != 1 || spec.GlobalFlags[0].Name != "ca-cert" {
		t.Fatalf("global flags = %+v, want [ca-cert]", spec.GlobalFlags)
	}

	// help + completion excluded; agent + component kept, sorted by name.
	if len(spec.Commands) != 2 {
		t.Fatalf("top-level commands = %d, want 2 (%+v)", len(spec.Commands), spec.Commands)
	}
	if spec.Commands[0].Path != "gibson agent" || spec.Commands[1].Path != "gibson component" {
		t.Fatalf("commands not sorted: %q, %q", spec.Commands[0].Path, spec.Commands[1].Path)
	}

	build := spec.Commands[1].Subcommands
	if len(build) != 1 || build[0].Path != "gibson component build" {
		t.Fatalf("nested subcommand = %+v", build)
	}
	if build[0].Long != "build long" {
		t.Fatalf("trailing newline not trimmed from long: %q", build[0].Long)
	}

	// The global flag must not be repeated on the subcommand; the boolean
	// false default is elided; the string default is kept.
	var force, output, caCert *FlagSpec
	for i := range build[0].Flags {
		switch build[0].Flags[i].Name {
		case "force":
			force = &build[0].Flags[i]
		case "output":
			output = &build[0].Flags[i]
		case "ca-cert":
			caCert = &build[0].Flags[i]
		}
	}
	if caCert != nil {
		t.Fatalf("global flag ca-cert leaked onto subcommand")
	}
	if force == nil || force.Default != "" {
		t.Fatalf("force flag default not elided: %+v", force)
	}
	if output == nil || output.Default != "out" {
		t.Fatalf("output flag default missing: %+v", output)
	}
}

// TestBuildCLISpec_Deterministic asserts the JSON encoding is byte-stable, so
// the docs-site drift gate never flaps on a re-run.
func TestBuildCLISpec_Deterministic(t *testing.T) {
	first, err := json.MarshalIndent(BuildCLISpec(syntheticTree()), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		next, err := json.MarshalIndent(BuildCLISpec(syntheticTree()), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(next, first) {
			t.Fatalf("non-deterministic spec on iteration %d", i)
		}
	}
}
