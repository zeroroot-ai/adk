// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package validate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/component"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/validate"
)

// saveComponentYAML marshals c and writes it to path as the test fixture
// component.yaml the validate package reads.
func saveComponentYAML(t *testing.T, path string, c *component.Component) {
	t.Helper()
	b, err := yaml.Marshal(c)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, b, 0o644))
}

func writeAgent(t *testing.T, dir, name string) {
	t.Helper()
	c := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindAgent,
		Metadata:   component.ComponentMetadata{Name: name, Version: "0.1.0"},
	}
	saveComponentYAML(t, filepath.Join(dir, "component.yaml"), c)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
}

func TestRun_AgentClean(t *testing.T) {
	dir := t.TempDir()
	writeAgent(t, dir, "demo")

	r, err := validate.Run(dir, "")
	require.NoError(t, err)
	assert.False(t, r.HasErrors(), "agent should be clean: %+v", r.Errors)
}

func TestRun_AgentMissingMainGo(t *testing.T) {
	dir := t.TempDir()
	c := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindAgent,
		Metadata:   component.ComponentMetadata{Name: "demo", Version: "0.1.0"},
	}
	saveComponentYAML(t, filepath.Join(dir, "component.yaml"), c)

	r, err := validate.Run(dir, "")
	require.NoError(t, err)
	assert.True(t, r.HasErrors())
	assert.Contains(t, r.Errors[0].Message, "main.go not found")
}

func TestRun_KindMismatch(t *testing.T) {
	dir := t.TempDir()
	writeAgent(t, dir, "demo")

	r, err := validate.Run(dir, component.KindTool)
	require.NoError(t, err)
	assert.True(t, r.HasErrors())
	assert.Contains(t, r.Errors[0].Message, "does not match")
}

func TestRun_PluginCleanManifest(t *testing.T) {
	dir := t.TempDir()
	c := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindPlugin,
		Metadata:   component.ComponentMetadata{Name: "demo-plugin", Version: "0.1.0"},
	}
	saveComponentYAML(t, filepath.Join(dir, "component.yaml"), c)

	manifest := `apiVersion: plugin.gibson.zeroroot.ai/v1
kind: Plugin
metadata:
  name: demo-plugin
  version: 0.1.0
spec:
  workload_class: plugin
  methods:
  - name: Echo
    description: "Echo returns the request message unchanged."
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644))

	r, err := validate.Run(dir, "")
	require.NoError(t, err)
	assert.False(t, r.HasErrors(), "plugin should be clean: %+v", r.Errors)
}

func TestRun_ToolMissingField100(t *testing.T) {
	dir := t.TempDir()
	c := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindTool,
		Metadata:   component.ComponentMetadata{Name: "demo-tool", Version: "0.1.0"},
	}
	saveComponentYAML(t, filepath.Join(dir, "component.yaml"), c)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))

	protoDir := filepath.Join(dir, "api", "proto", "gibson", "tools", "demotool", "v1")
	require.NoError(t, os.MkdirAll(protoDir, 0o755))
	bad := `syntax = "proto3";
package gibson.tools.demo.v1;
message DemoToolRequest { string target = 1; }
message DemoToolResponse { string raw = 1; }
`
	require.NoError(t, os.WriteFile(filepath.Join(protoDir, "demotool.proto"), []byte(bad), 0o644))

	r, err := validate.Run(dir, "")
	require.NoError(t, err)
	assert.True(t, r.HasErrors())
	found := false
	for _, e := range r.Errors {
		if strings.Contains(e.Message, "field 100") {
			found = true
		}
	}
	assert.True(t, found, "expected an error about field 100; got %+v", r.Errors)
}

func TestRun_ToolWithField100Passes(t *testing.T) {
	dir := t.TempDir()
	c := &component.Component{
		APIVersion: component.APIVersionV1,
		Kind:       component.KindTool,
		Metadata:   component.ComponentMetadata{Name: "demo-tool", Version: "0.1.0"},
	}
	saveComponentYAML(t, filepath.Join(dir, "component.yaml"), c)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))

	protoDir := filepath.Join(dir, "api", "proto", "gibson", "tools", "demotool", "v1")
	require.NoError(t, os.MkdirAll(protoDir, 0o755))
	good := `syntax = "proto3";
package gibson.tools.demo.v1;
import "gibson/graphrag/v1/graphrag.proto";
message DemoToolRequest { string target = 1; }
message DemoToolResponse {
  string raw = 1;
  gibson.graphrag.v1.DiscoveryResult discovery = 100;
}
`
	require.NoError(t, os.WriteFile(filepath.Join(protoDir, "demotool.proto"), []byte(good), 0o644))

	r, err := validate.Run(dir, "")
	require.NoError(t, err)
	// Allow buf-not-on-PATH to remain a warning; field 100 must not be an error.
	for _, e := range r.Errors {
		assert.NotContains(t, e.Message, "field 100", "field 100 must validate cleanly when present")
	}
}
