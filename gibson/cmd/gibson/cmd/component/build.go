// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
  3. proto     — run buf generate when buf.gen.yaml exists and api/gen
                 is missing or older than api/proto (needs buf,
                 protoc-gen-go and protoc-gen-go-grpc on PATH)
  4. mod tidy  — populate go.sum on a freshly scaffolded component
                 (skipped once go.sum exists)
  5. go build  — compile the component binary into the component directory

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
//  3. proto bindings (buf generate, when needed)
//  4. go build ./...
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

	// Step 3 — proto bindings. A tool's main.go imports api/gen/..., which
	// only buf generate creates. Without it `go mod tidy` fails to resolve
	// the import, so generate before resolving modules.
	if err := ensureProtoBindings(abs); err != nil {
		return err
	}

	// Step 4 — resolve modules. A freshly scaffolded component ships a
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

	// Step 5 — go build ./...
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

// ensureProtoBindings runs `buf generate` when the component has a
// buf.gen.yaml and its generated Go package (api/gen) is missing or older
// than the newest .proto under api/proto. A component without buf.gen.yaml
// needs no bindings, so it is skipped.
func ensureProtoBindings(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "buf.gen.yaml")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("component build: stat buf.gen.yaml: %w", err)
	}
	protoTime, err := newestFile(filepath.Join(dir, "api", "proto"), ".proto")
	if err != nil {
		return err
	}
	genTime, err := newestFile(filepath.Join(dir, "api", "gen"), ".go")
	if err != nil {
		return err
	}
	if !genTime.IsZero() && !genTime.Before(protoTime) {
		return nil
	}

	buf, err := exec.LookPath("buf")
	if err != nil {
		return errors.New("component build: api/gen is missing or stale and buf is not on PATH: " +
			"install buf (https://buf.build/docs/installation), protoc-gen-go and protoc-gen-go-grpc, then run `make proto`")
	}
	fmt.Println("component build: api/gen is missing or stale — running buf generate...")
	gen := exec.Command(buf, "generate")
	gen.Dir = dir
	gen.Stdout = os.Stdout
	gen.Stderr = os.Stderr
	if err := gen.Run(); err != nil {
		return fmt.Errorf("component build: buf generate failed (check protoc-gen-go and protoc-gen-go-grpc are on PATH, then run `make proto`): %w", err)
	}
	return nil
}

// newestFile returns the latest modification time of a file under root
// whose name ends in suffix. It returns the zero time when root does not
// exist or holds no such file.
func newestFile(root, suffix string) (time.Time, error) {
	var newest time.Time
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, suffix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, fmt.Errorf("component build: scan %s: %w", root, err)
	}
	return newest, nil
}
