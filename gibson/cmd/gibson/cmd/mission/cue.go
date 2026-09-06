// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
)

// compileCUE compiles raw CUE bytes without import resolution.
// Used as a fast path for CUE files that contain no import statements.
// Errors carry the CUE library's source position information.
func compileCUE(src []byte) (*cue.Context, cue.Value, error) {
	ctx := cuecontext.New()
	expr, err := parser.ParseFile("mission.cue", src, parser.ParseComments)
	if err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue parse: %w", err)
	}
	v := ctx.BuildFile(expr)
	if err := v.Err(); err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue build: %w", err)
	}
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue validate: %w", err)
	}
	return ctx, v, nil
}

// compileCUEWithSchema compiles raw CUE bytes that may import
// "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1",
// resolves imports against the embedded schema bundle, and validates
// the result structurally against #MissionDefinition.
//
// Import resolution works via cue/load with an overlay containing:
//   - The embedded cue.mod/module.cue (declares "github.com/zeroroot-ai/sdk@v1")
//   - The mission schema package (api/proto/gibson/mission/v1)
//   - A typespb stub (api/proto/gibson/types/v1) fixing the broken import
//     alias in the upstream generated CUE (tracked in sdk#48)
//
// The user's CUE bytes are registered in the overlay alongside the
// schema files, then loaded by absolute path. This means import
// resolution works without the sibling opensource/sdk/ checkout or
// any network fetch — the schema is baked into the adk binary via
// //go:embed in schema.go.
//
// Error ordering guarantee: structural errors (unknown fields, type
// mismatches) surface as "cue build: ..." errors before the
// protojson.Unmarshal step in parseCUE.
func compileCUEWithSchema(src []byte) (*cue.Context, cue.Value, error) {
	overlay, err := buildSchemaOverlay()
	if err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue schema overlay: %w", err)
	}

	// Register the user's template in the overlay at a stable absolute path
	// inside the schema module root. The path must be under ModuleRoot so
	// that cue/load can trace it back to the module when resolving imports.
	userFilePath := schemaModuleRoot + "/user/mission.cue"
	overlay[userFilePath] = load.FromBytes(src)

	cfg := &load.Config{
		Dir:        schemaModuleRoot + "/user",
		ModuleRoot: schemaModuleRoot,
		Overlay:    overlay,
		// Package "_" loads CUE files without a package clause (anonymous).
		Package: "_",
	}

	ctx := cuecontext.New()

	// Load the user file by its absolute overlay path. Using the path
	// directly (not ".") avoids ambiguity when Package "_" is set.
	insts := load.Instances([]string{userFilePath}, cfg)
	if len(insts) == 0 {
		return nil, cue.Value{}, errors.New("cue load: no instances")
	}
	inst := insts[0]
	if inst.Err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue build: %w", inst.Err)
	}

	v := ctx.BuildInstance(inst)
	if err := v.Err(); err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue build: %w", err)
	}

	// Validate catches any remaining structural errors (open constraints,
	// disjunction conflicts) that Build didn't surface yet.
	if err := v.Validate(cue.Concrete(false)); err != nil {
		return nil, cue.Value{}, fmt.Errorf("cue build: %w", err)
	}

	return ctx, v, nil
}

// hasImports reports whether src contains an import declaration.
// Used to route between the fast no-import path and the overlay path.
func hasImports(src []byte) bool {
	f, err := parser.ParseFile("mission.cue", src, parser.ParseComments)
	if err != nil {
		return false
	}
	return len(f.Imports) > 0
}

// cuePath returns a cue.Path for the named field. Used by the
// mission loader to look up the conventional `mission:` key.
func cuePath(_ *cue.Context, name string) cue.Path {
	return cue.MakePath(cue.Str(name))
}
