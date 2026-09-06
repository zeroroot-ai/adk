// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package docs implements the `gibson docs` verb group.
package docs

import "github.com/spf13/cobra"

// Command returns the root `gibson docs` cobra command.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Emit machine-readable docs (JSON Schemas, etc.)",
		Long: `docs — emit developer-facing reference material.

Subcommands:
  schema    JSON Schema (Draft 2020-12) for component.yaml / plugin.yaml
  cli       the command tree as machine-readable JSON (drives the CLI reference)`,
	}
	cmd.AddCommand(schemaCmd())
	cmd.AddCommand(cliCmd())
	return cmd
}
