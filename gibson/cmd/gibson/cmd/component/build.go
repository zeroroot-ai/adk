// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// buildCmd returns `gibson component build`.
func buildCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Generate, validate, and compile the component binary",
		Long: `build is the one-step developer loop command:

  1. generate  — regenerate gen/ from taxonomy.yaml + ontology.yaml
  2. validate  — run all local checks (component.yaml, proto field 100,
                 buf lint, ontology YAML parse)
  3. mod tidy  — populate go.sum on a freshly scaffolded component
                 (skipped once go.sum exists)
  4. go build  — compile the component binary into the component directory

build delegates to the generate and validate subcommands, resolves
modules when go.sum is absent, then runs ` + "`go build ./...`" + ` in the
component directory. Use ` + "`gibson component generate`" + `
or ` + "`gibson component validate`" + ` individually if you want finer control.

Exit codes:
  0  build succeeded
  1  generate / validate / compile error (details on stderr)

Examples:
  gibson component build
  gibson component build --dir ./my-tool`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runBuild(dir)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory (containing component.yaml)")
	return cmd
}

// runBuild implements `gibson component build`:
//  1. generate (ontology codegen)
//  2. validate (all local checks)
//  3. go build ./...
func runBuild(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("component build: resolve dir: %w", err)
	}

	// Step 1 — generate.
	fmt.Println("component build: running generate...")
	if err := runGenerate(abs); err != nil {
		return err
	}

	// Step 2 — validate.
	fmt.Println("component build: running validate...")
	if err := runValidate(abs, "" /*auto-detect kind*/); err != nil {
		return err
	}

	// Step 3 — resolve modules. A freshly scaffolded component ships a
	// go.mod with only the direct SDK require and no go.sum, so a bare
	// `go build` fails with "missing go.sum entry". Run `go mod tidy` to
	// populate go.sum and the indirect requires before compiling. We only
	// do this on the first build (no go.sum yet) so an already-resolved
	// component's curated go.mod is left untouched.
	if _, err := os.Stat(filepath.Join(abs, "go.sum")); os.IsNotExist(err) {
		fmt.Println("component build: no go.sum — running go mod tidy...")
		tidy := exec.Command("go", "mod", "tidy")
		tidy.Dir = abs
		tidy.Stdout = os.Stdout
		tidy.Stderr = os.Stderr
		if err := tidy.Run(); err != nil {
			return fmt.Errorf("component build: go mod tidy failed: %w", err)
		}
	}

	// Step 4 — go build ./...
	fmt.Println("component build: running go build ./...")
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = abs
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("component build: go build failed: %w", err)
	}

	fmt.Println("component build: OK")
	return nil
}
