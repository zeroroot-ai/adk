// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package schema emits JSON Schema (Draft 2020-12) for component.yaml
// and plugin.yaml. Schemas are committed as goldens under
// testdata/golden/ and embedded into the binary at compile time, so
// the runtime command (gibson docs schema) emits deterministic bytes
// without reflection in production. A unit test diffs the generated
// vs the golden to catch drift when the underlying Go types change.
package schema

import (
	_ "embed"
)

//go:embed testdata/golden/component-yaml.schema.json
var componentYAMLBytes []byte

//go:embed testdata/golden/plugin-yaml.schema.json
var pluginYAMLBytes []byte

// ComponentYAMLSchema returns the JSON Schema (Draft 2020-12) for
// component.yaml. The bytes come from the committed golden file at
// testdata/golden/component-yaml.schema.json.
func ComponentYAMLSchema() []byte { return componentYAMLBytes }

// PluginYAMLSchema returns the JSON Schema (Draft 2020-12) for
// plugin.yaml. The bytes come from the committed golden file at
// testdata/golden/plugin-yaml.schema.json.
func PluginYAMLSchema() []byte { return pluginYAMLBytes }

// Available reports the schema names the gibson docs schema verb
// supports. Stable order for deterministic help output.
func Available() []string {
	return []string{"component-yaml", "plugin-yaml"}
}

// Lookup returns the schema bytes by name (one of Available()).
// Returns nil for unknown names.
func Lookup(name string) []byte {
	switch name {
	case "component-yaml":
		return ComponentYAMLSchema()
	case "plugin-yaml":
		return PluginYAMLSchema()
	}
	return nil
}
