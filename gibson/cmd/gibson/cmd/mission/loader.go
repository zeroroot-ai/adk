// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	missionv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/mission/v1"
	"google.golang.org/protobuf/encoding/protojson"
	sigsyaml "sigs.k8s.io/yaml"
)

// loadMissionFile reads a mission file from disk and returns the
// parsed *missionv1.MissionDefinition. The format is detected from
// the file extension (.cue, .yaml, .yml, .json). Reading from
// stdin is supported via path "-"; in that case the format must be
// passed via the --format flag (default "yaml").
func loadMissionFile(path, formatHint string) (*missionv1.MissionDefinition, error) {
	src, format, err := readSource(path, formatHint)
	if err != nil {
		return nil, err
	}
	return parseMission(src, format)
}

func readSource(path, formatHint string) ([]byte, string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, "", fmt.Errorf("read stdin: %w", err)
		}
		f := formatHint
		if f == "" {
			f = "yaml"
		}
		return data, f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "cue":
		return data, "cue", nil
	case "yaml", "yml":
		return data, "yaml", nil
	case "json":
		return data, "json", nil
	}
	if formatHint != "" {
		return data, formatHint, nil
	}
	return nil, "", fmt.Errorf("cannot infer format from %q; pass --format yaml|json|cue", path)
}

func parseMission(src []byte, format string) (*missionv1.MissionDefinition, error) {
	switch format {
	case "json":
		return parseJSON(src)
	case "yaml", "yml":
		return parseYAML(src)
	case "cue":
		return parseCUE(src)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func parseYAML(src []byte) (*missionv1.MissionDefinition, error) {
	jsonBytes, err := sigsyaml.YAMLToJSON(src)
	if err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return parseJSON(jsonBytes)
}

func parseJSON(src []byte) (*missionv1.MissionDefinition, error) {
	def := &missionv1.MissionDefinition{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(src, def); err != nil {
		return nil, fmt.Errorf("protojson: %w", err)
	}
	return def, nil
}

// parseCUE compiles a CUE document, validates its structure against the
// embedded #MissionDefinition schema when the file contains import
// statements (catching unknown fields and type mismatches before
// protojson), emits proto-shaped JSON, then routes through parseJSON.
//
// Two compilation paths:
//   - Files with import statements use compileCUEWithSchema, which
//     resolves "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"
//     against the schema bundle embedded in the binary (see schema.go).
//     This is the schema-aware path introduced in adk#20.
//   - Files without imports use compileCUE (fast path; supports inline
//     CUE authoring without the import boilerplate).
//
// Error ordering guarantee: for import-using files, structural CUE
// errors (unknown fields, incompatible types) surface as "cue build:
// ..." errors before the protojson.Unmarshal step.
func parseCUE(src []byte) (*missionv1.MissionDefinition, error) {
	var (
		ctx      *cue.Context
		instance cue.Value
		err      error
	)
	if hasImports(src) {
		ctx, instance, err = compileCUEWithSchema(src)
	} else {
		ctx, instance, err = compileCUE(src)
	}
	if err != nil {
		return nil, err
	}
	value := instance.LookupPath(cuePath(ctx, "mission"))
	if !value.Exists() {
		// Fall back to root value (whole file is a mission).
		value = instance
	}
	jsonBytes, err := value.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("cue marshal: %w", err)
	}
	// Strip trailing newline/whitespace before handing to protojson.
	return parseJSON(bytes.TrimSpace(jsonBytes))
}
