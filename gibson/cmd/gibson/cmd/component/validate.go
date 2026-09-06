// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/validate"
)

// validateCmd returns `gibson component validate`.
func validateCmd() *cobra.Command {
	var (
		dir  string
		kind string
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Local schema + proto checks against component.yaml / plugin.yaml",
		Long: `validate runs kind-aware local checks against the component in --dir
(default: current directory). The kind is auto-detected from
component.yaml; pass --kind to override.

  agent:  component.yaml shape, main.go parses
  tool:   agent checks, plus proto field 100 = DiscoveryResult, plus
          buf lint when buf is on PATH
  plugin: agent checks, plus the SDK manifest validator

Exit codes:
  0  no errors
  2  validation errors (one or more findings printed to stderr)
  1  I/O / setup error (e.g. component.yaml missing)

Examples:
  gibson component validate
  gibson component validate --dir ./my-tool
  gibson component validate --kind plugin   # override auto-detect`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runValidate(dir, kind)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory (containing component.yaml)")
	cmd.Flags().StringVar(&kind, "kind", "", "override kind: agent | tool | plugin")
	return cmd
}

func runValidate(dir, kindStr string) error {
	report, err := validate.Run(dir, component.Kind(kindStr))
	if err != nil {
		return err // I/O / setup error -> exit 1
	}

	for _, w := range report.Warnings {
		fmt.Fprintf(os.Stderr, "WARN %s\n", w.String())
	}
	for _, e := range report.Errors {
		fmt.Fprintln(os.Stderr, e.String())
	}

	if report.HasErrors() {
		fmt.Fprintf(os.Stderr, "\nvalidation failed: %d error(s)\n", len(report.Errors))
		return ExitCodeError{Code: 2}
	}
	fmt.Println("validate: OK")
	return nil
}
