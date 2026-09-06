// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"embed"
	"io/fs"
	"path"
	"path/filepath"

	"cuelang.org/go/cue/load"
)

// schemaFS holds the embedded CUE module that provides
// github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1.
//
// The bundle is a minimal CUE module containing:
//   - cue.mod/module.cue          — declares "github.com/zeroroot-ai/sdk@v1"
//   - api/proto/gibson/mission/v1 — #MissionDefinition and related types
//   - api/proto/gibson/types/v1   — typespb stub (TaskConstraints and Task)
//   - api/proto/gibson/job/v1     — jobpb stub (JobSpec and its nested types)
//
// The bundle mirrors the SDK's api/proto tree, so a template imports a
// package at the same path it has in the SDK.
//
// Using //go:embed means schema validation works without the sibling
// opensource/sdk/ checkout and without a network fetch. The bundle is
// pinned to the adk module version via normal Go module semantics.
//
//go:embed schema
var schemaFS embed.FS

// schemaModuleRoot is the virtual root used for the overlay.
// The path is arbitrary but must be absolute-looking for cue/load.
const schemaModuleRoot = "/cue-schema-root"

// buildSchemaOverlay returns a cue/load overlay that registers all files
// from the embedded schema FS under schemaModuleRoot.
//
// The overlay maps every embedded file to its absolute virtual path so
// that cue/load can resolve imports within the schema module without
// touching the host filesystem.
func buildSchemaOverlay() (map[string]load.Source, error) {
	overlay := make(map[string]load.Source)
	err := fs.WalkDir(schemaFS, "schema", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := schemaFS.ReadFile(p)
		if err != nil {
			return err
		}
		// Strip the leading "schema/" prefix and prepend the virtual root.
		rel := p[len("schema/"):]
		abs := path.Join(schemaModuleRoot, filepath.ToSlash(rel))
		overlay[abs] = load.FromBytes(data)
		return nil
	})
	return overlay, err
}
