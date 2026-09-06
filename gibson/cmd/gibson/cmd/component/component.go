// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package component implements the unified `gibson component` verb
// group: init, generate, build, validate, register, run.
//
// Each subcommand is kind-aware: when the working directory contains a
// component.yaml, the kind is auto-detected; otherwise --kind is
// required. This is the single verb surface for the agent / tool /
// plugin developer workflow.
package component

import "github.com/spf13/cobra"

// Command returns the root `component` cobra command.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component",
		Short: "Scaffold, validate, register, and run Gibson components (agent | tool | plugin | connector)",
		Long: `component — kind-aware tooling for the Gibson developer workflow.

Subcommands:
  init      scaffold a new component directory from templates
  generate  regenerate gen/ from taxonomy.yaml and ontology.yaml
  build     generate + validate + go build (one-step developer loop)
  validate  local schema + proto checks against component.yaml / plugin.yaml
  register  consume a dashboard-issued enroll_command (no admin RPC auto-mint)
  run       run the compiled component binary, supervising signals and exit code 75

In a directory containing a component.yaml, --kind is auto-detected
from the file. Outside such a directory, --kind is required.`,
	}
	cmd.AddCommand(initCmd())
	cmd.AddCommand(generateCmd())
	cmd.AddCommand(buildCmd())
	cmd.AddCommand(validateCmd())
	cmd.AddCommand(registerCmd())
	cmd.AddCommand(runCmd())
	return cmd
}
