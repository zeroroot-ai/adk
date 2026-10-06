// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

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
		Short: "Local schema + proto checks against a component directory",
		Long: `validate runs kind-aware local checks against the component in --dir
(default: current directory).

--kind is required. It used to be auto-detected from component.yaml, which
no longer exists (ADR-0097): the kind is the only thing a component
directory cannot imply, so it is the one thing you still state.

  agent:  main.go is present and parses
  tool:   agent checks, plus proto field 100 = DiscoveryResult, plus
          buf lint when buf is on PATH
  plugin: Go source of package main at the root, and each file parses;
          the plugin declares itself in code (ADR-0097)

Paths come from the directory rather than from a manifest: main.go at the
root, and the tool proto at
api/proto/gibson/tools/<name>/v1/<name>.proto, with <name> the directory
name minus hyphens.

Exit codes:
  0  no errors
  2  validation errors (one or more findings printed to stderr)
  1  I/O / setup error (e.g. --kind missing, or an unusable directory name)

Examples:
  gibson component validate --kind tool
  gibson component validate --kind plugin --dir ./my-plugin`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runValidate(dir, kind)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory")
	cmd.Flags().StringVar(&kind, "kind", "", "component kind: "+strings.Join(validate.Kinds(), " | ")+" (required)")
	return cmd
}

func runValidate(dir, kindStr string) error {
	report, err := validate.Run(dir, kindStr)
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
