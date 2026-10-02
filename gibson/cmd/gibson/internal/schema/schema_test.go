// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/schema"
)

func TestPluginYAMLSchema_IsValidJSON(t *testing.T) {
	b := schema.PluginYAMLSchema()
	require.NotEmpty(t, b)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc), "plugin-yaml.schema.json must parse as JSON")

	assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", doc["$schema"])
	assert.Equal(t, "https://schemas.zeroroot.ai/plugin-yaml-v1.json", doc["$id"])
	assert.Equal(t, "plugin.yaml", doc["title"])
}

func TestAvailable_StableOrder(t *testing.T) {
	got := schema.Available()
	assert.Equal(t, []string{"plugin-yaml"}, got)
}

func TestLookup(t *testing.T) {
	assert.NotEmpty(t, schema.Lookup("plugin-yaml"))
	assert.Nil(t, schema.Lookup("nonsense"))
}

func TestPluginYAMLSchema_StructIsConsistent(t *testing.T) {
	var doc map[string]any
	require.NoError(t, json.Unmarshal(schema.PluginYAMLSchema(), &doc))

	props := doc["properties"].(map[string]any)
	for _, k := range []string{"apiVersion", "kind", "metadata", "spec"} {
		assert.Contains(t, props, k, "plugin-yaml schema missing top-level property %q", k)
	}

	spec := props["spec"].(map[string]any)
	specProps := spec["properties"].(map[string]any)
	for _, k := range []string{"workload_class", "methods", "secrets", "runtime", "health", "egress"} {
		assert.Contains(t, specProps, k, "plugin-yaml spec missing %q", k)
	}
}
