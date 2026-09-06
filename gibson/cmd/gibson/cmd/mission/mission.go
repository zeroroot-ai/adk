// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package mission groups the `gibson mission` subcommands:
// new, validate, render, submit. (The `draft` subcommand was removed
// in the E6 narrow-SDK flip — it called TenantService, which moved out
// of the OSS SDK into the gibson daemon-local proto tree and is no
// longer a component-developer surface. ADR-0058 amendment, adk#155.)
// Together they implement the
// authoring path from the mission-authoring-cue spec — authors
// write CUE (or YAML / JSON), the CLI compiles to proto-shaped
// JSON, the daemon receives canonical proto.
//
// In v1 the CLI accepts CUE, YAML, and JSON — CUE is the
// recommended authoring format for power users; YAML is the
// dashboard's serialization; JSON is the raw exchange format.
// `cue` evaluation is done out-of-process via the cuelang.org/go
// library when the input is .cue; YAML and JSON paths use
// sigs.k8s.io/yaml + protojson directly.
//
// Spec: mission-authoring-cue Requirements 3, 4, 5, 6.
package mission

import (
	"github.com/spf13/cobra"
)

// Command returns the `gibson mission` parent command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "mission",
		Short: "Author, validate, render, and submit gibson missions",
		Long: `gibson mission — author missions in CUE / YAML / JSON
and submit them to the daemon.

Subcommands:
  new       scaffold a new mission (--from-template <name> | minimal)
  validate  run cue vet / proto validation against an input file
  render    compile the input to proto-shaped JSON or YAML
  submit    validate + define + run via the daemon (full CUE→define→run)`,
		SilenceUsage: true,
	}
	c.AddCommand(newCmd())
	c.AddCommand(validateCmd())
	c.AddCommand(renderCmd())
	c.AddCommand(submitCmd())
	return c
}
