// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/schema"
)

func schemaCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "schema [name]",
		Short: "Emit JSON Schema for component.yaml / plugin.yaml",
		Long: `schema emits the JSON Schema (Draft 2020-12) for a Gibson YAML
shape. With no name, lists available schemas. With one of the
supported names, writes the schema to stdout (or --output <dir>).

Editor and AI-coder integration: pipe the output into your project's
schema config (e.g. yaml-language-server settings, JetBrains schema
registry) and your editor will validate component.yaml / plugin.yaml
inline.

Examples:
  gibson docs schema                       # list available schemas
  gibson docs schema component-yaml        # emit to stdout
  gibson docs schema plugin-yaml | jq .    # validate JSON
  gibson docs schema --output ./schemas    # write both schemas to disk`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runSchema(args, output)
		},
	}

	cmd.Flags().StringVar(&output, "output", "", "directory to write *.schema.json into (writes both schemas)")
	return cmd
}

func runSchema(args []string, outputDir string) error {
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("docs schema: create output dir: %w", err)
		}
		for _, name := range schema.Available() {
			b := schema.Lookup(name)
			path := filepath.Join(outputDir, name+".schema.json")
			if err := os.WriteFile(path, b, 0o644); err != nil {
				return fmt.Errorf("docs schema: write %s: %w", path, err)
			}
			fmt.Println(path)
		}
		return nil
	}

	if len(args) == 0 {
		fmt.Println("Available schemas:")
		for _, name := range schema.Available() {
			fmt.Println("  " + name)
		}
		fmt.Println("\nRun `gibson docs schema <name>` to emit a schema, or pass --output <dir>.")
		return nil
	}

	name := strings.TrimSpace(args[0])
	b := schema.Lookup(name)
	if b == nil {
		return fmt.Errorf("docs schema: unknown schema %q (available: %s)", name, strings.Join(schema.Available(), ", "))
	}
	if _, err := os.Stdout.Write(b); err != nil {
		return fmt.Errorf("docs schema: write stdout: %w", err)
	}
	return nil
}
