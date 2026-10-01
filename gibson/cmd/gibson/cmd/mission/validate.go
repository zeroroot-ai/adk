// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"fmt"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// checkTargetRef applies the daemon's target rule: CreateMission accepts a
// target UUID and nothing else, and answers InvalidArgument otherwise. An
// empty ref passes, because submit can take the target from --target.
func checkTargetRef(ref string) error {
	if ref == "" {
		return nil
	}
	if _, err := uuid.Parse(ref); err != nil {
		return fmt.Errorf("target_ref %q is not a target UUID: set it to the id of a target "+
			"(see `gibson target create` and `gibson target list`), or leave it empty and pass --target <uuid> to submit", ref)
	}
	return nil
}

func validateCmd() *cobra.Command {
	var formatHint string
	c := &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a mission file (CUE / YAML / JSON)",
		Long: `Validate a mission file.

Steps:
1. Read and parse the file (CUE → JSON via cuelang; YAML → JSON via
   sigs.k8s.io/yaml; JSON passes through). Format detected from the
   file extension; override with --format.
2. Unmarshal into *missionv1.MissionDefinition via protojson.
3. Run protovalidate against the message — every
   (buf.validate.field).* annotation declared in the SDK protos
   is enforced here, matching what the daemon applies at submit
   time.
4. Check that target_ref, when set, is a target UUID. The daemon
   rejects any other value.

Exits non-zero with the underlying library's error message on any
failure. Use '-' as the file path to read from stdin.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := loadMissionFile(args[0], formatHint)
			if err != nil {
				return err
			}
			v, err := protovalidate.New()
			if err != nil {
				return fmt.Errorf("protovalidate.New: %w", err)
			}
			if err := v.Validate(def); err != nil {
				return fmt.Errorf("validate: %w", err)
			}
			if err := checkTargetRef(def.GetTargetRef()); err != nil {
				return fmt.Errorf("validate: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
	c.Flags().StringVar(&formatHint, "format", "", "Override format detection: cue|yaml|json")
	return c
}
