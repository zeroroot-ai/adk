// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/sdk/taxonomy"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/scaffold"
)

// generateCmd returns `gibson component generate`.
func generateCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Go bindings from taxonomy.yaml and ontology.yaml",
		Long: `generate reads taxonomy.yaml and ontology.yaml (if present) in the
component directory and emits Go source files under gen/:

  gen/ontology_extension.go  — exports OntologyExtension() returning a
                               graphrag.OntologyExtension populated from
                               the parsed ontology.yaml

Generated files are byte-stable across runs: map keys are sorted and
go/format is applied. Commit the gen/ directory; diffs reflect only
intentional ontology changes.

generate is a no-op if neither YAML file is present (it prints a notice
and exits 0 rather than failing).

Examples:
  gibson component generate
  gibson component generate --dir ./my-tool`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runGenerate(dir)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "component directory (containing component.yaml)")
	return cmd
}

// runGenerate implements `gibson component generate`.
func runGenerate(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("component generate: resolve dir: %w", err)
	}

	generated := false

	// ontology.yaml — optional.
	ontologyPath := filepath.Join(abs, "ontology.yaml")
	if _, err := os.Stat(ontologyPath); err == nil {
		if err := generateOntology(abs, ontologyPath); err != nil {
			return err
		}
		fmt.Printf("component generate: wrote gen/ontology_extension.go from %s\n", ontologyPath)
		generated = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("component generate: stat %s: %w", ontologyPath, err)
	}

	if !generated {
		fmt.Println("component generate: no taxonomy.yaml or ontology.yaml found — nothing to generate")
	}
	return nil
}

// generateOntology parses ontologyPath and runs the codegen into outDir/gen/.
func generateOntology(outDir, ontologyPath string) error {
	b, err := os.ReadFile(ontologyPath)
	if err != nil {
		return fmt.Errorf("component generate: read %s: %w", ontologyPath, err)
	}
	ont, err := taxonomy.Parse(b)
	if err != nil {
		return fmt.Errorf("component generate: parse %s: %w", ontologyPath, err)
	}
	if err := scaffold.Generate(ont, outDir); err != nil {
		return fmt.Errorf("component generate: %w", err)
	}
	return nil
}
