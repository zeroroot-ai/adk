// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	sigsyaml "sigs.k8s.io/yaml"
)

func renderCmd() *cobra.Command {
	var (
		formatHint string
		outFormat  string
	)
	c := &cobra.Command{
		Use:   "render <file>",
		Short: "Compile a mission file to proto-shaped JSON or YAML",
		Long: `Render the input mission file as the canonical proto-shaped JSON
the daemon expects. With --out-format yaml, JSON is converted to YAML
via sigs.k8s.io/yaml (round-trip-equivalent).

Useful for:
- Reviewing what the daemon will see before submitting.
- Diffing two mission files semantically (compile both then diff JSON).
- Piping into other tooling (` + "`gibson mission render m.cue | jq …`" + `).

Output is deterministic: protojson controls proto field ordering (camelCase,
stable); encoding/json.Indent normalises whitespace, eliminating the
binary-hash-seeded extra space that protojson's internal detrand inserts
after colons to make builds non-reproducible.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := loadMissionFile(args[0], formatHint)
			if err != nil {
				return err
			}
			// EmitUnpopulated: false (default) — omit zero-value fields so the
			// JSON stays minimal.  UseProtoNames: false (default) — camelCase
			// output matches what the daemon and dashboard expect.
			//
			// We do NOT pass Multiline/Indent to protojson because its
			// internal detrand package intentionally seeds whitespace from a
			// hash of the compiled binary, making the exact spacing non-
			// reproducible across builds.  Instead we marshal compact and
			// then re-indent through encoding/json, which is deterministic.
			marshalOpts := protojson.MarshalOptions{}
			compactBytes, err := marshalOpts.Marshal(def)
			if err != nil {
				return fmt.Errorf("protojson marshal: %w", err)
			}
			// Re-indent via stdlib JSON: deterministic "  " indent, single
			// space after colon, consistent across every build and platform.
			var jsonBytes bytes.Buffer
			if err := json.Indent(&jsonBytes, compactBytes, "", "  "); err != nil {
				return fmt.Errorf("json indent: %w", err)
			}
			switch outFormat {
			case "json", "":
				fmt.Fprintln(cmd.OutOrStdout(), jsonBytes.String())
			case "yaml":
				yamlBytes, err := sigsyaml.JSONToYAML(jsonBytes.Bytes())
				if err != nil {
					return fmt.Errorf("yaml convert: %w", err)
				}
				fmt.Fprint(cmd.OutOrStdout(), string(yamlBytes))
			default:
				return fmt.Errorf("unsupported --out-format %q (use json or yaml)", outFormat)
			}
			return nil
		},
	}
	c.Flags().StringVar(&formatHint, "format", "", "Override input format detection: cue|yaml|json")
	c.Flags().StringVar(&outFormat, "out-format", "json", "Output format: json|yaml")
	return c
}
