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

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/validate"
)

// These tests used to write a component.yaml fixture into a random temp
// directory, because the kind and the component name were read from the file.
// With the file gone (ADR-0097, adk#90) the directory IS the fixture: it is
// named after the component, and the kind is passed in. So every case now
// creates a NAMED directory, which is also the shape a developer's checkout has.

// componentDir creates <t.TempDir()>/<name>, which is what layout.Name reads the
// component name from.
func componentDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	// #nosec G301 -- a test fixture directory under t.TempDir(), which the test
	// framework removes; the mode only has to let this process traverse it.
	require.NoError(t, os.MkdirAll(dir, 0o750))
	return dir
}

func writeMainGo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
}

func TestRun_AgentClean(t *testing.T) {
	dir := componentDir(t, "demo")
	writeMainGo(t, dir)

	r, err := validate.Run(dir, validate.KindAgent)
	require.NoError(t, err)
	assert.False(t, r.HasErrors(), "agent should be clean: %+v", r.Errors)
}

func TestRun_AgentMissingMainGo(t *testing.T) {
	dir := componentDir(t, "demo")

	r, err := validate.Run(dir, validate.KindAgent)
	require.NoError(t, err)
	assert.True(t, r.HasErrors())
	assert.Contains(t, r.Errors[0].Message, "main.go not found")
}

// TestRun_KindIsRequired replaces TestRun_KindMismatch. There is no longer a
// declared kind to disagree with, so the failure mode moved: an absent kind is a
// setup error, not a finding about the component. Reporting it as a finding would
// say the component is wrong when the invocation is.
func TestRun_KindIsRequired(t *testing.T) {
	dir := componentDir(t, "demo")
	writeMainGo(t, dir)

	_, err := validate.Run(dir, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--kind is required")
	assert.Contains(t, err.Error(), "component.yaml", "the error should say where the kind used to come from")
}

func TestRun_UnknownKindIsASetupError(t *testing.T) {
	dir := componentDir(t, "demo")
	writeMainGo(t, dir)

	_, err := validate.Run(dir, "sidecar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent|tool|plugin")
}

// TestRun_UnusableDirectoryName. The component name is the directory name, so a
// directory that cannot be a DNS label cannot be a component — and saying so is
// better than deriving a proto path nobody will ever have.
func TestRun_UnusableDirectoryName(t *testing.T) {
	dir := componentDir(t, "Demo_Tool")
	writeMainGo(t, dir)

	_, err := validate.Run(dir, validate.KindTool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a usable component name")
}

// TestRun_PluginNeedsNoManifest: a plugin declares itself in code
// (ADR-0097), so a directory with Go source of package main and no
// plugin.yaml is clean.
func TestRun_PluginNeedsNoManifest(t *testing.T) {
	dir := componentDir(t, "demo-plugin")
	// #nosec G306 -- a Go fixture under t.TempDir(); only this test reads it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))

	r, err := validate.Run(dir, validate.KindPlugin)
	require.NoError(t, err)
	assert.False(t, r.HasErrors(), "plugin should be clean: %+v", r.Errors)
}

// TestRun_PluginWithoutGoSourceFails: an empty directory is no plugin.
func TestRun_PluginWithoutGoSourceFails(t *testing.T) {
	dir := componentDir(t, "demo-plugin")

	r, err := validate.Run(dir, validate.KindPlugin)
	require.NoError(t, err)
	require.True(t, r.HasErrors())
	assert.Contains(t, r.Errors[0].Message, "no Go file of package main")
}

// writeToolProto writes a tool's proto at the path derived from the directory
// name: api/proto/gibson/tools/<name-without-hyphens>/v1/<same>.proto.
func writeToolProto(t *testing.T, dir, pkg, body string) {
	t.Helper()
	protoDir := filepath.Join(dir, "api", "proto", "gibson", "tools", pkg, "v1")
	require.NoError(t, os.MkdirAll(protoDir, 0o755))
	// #nosec G306 -- a .proto fixture under t.TempDir(); nothing reads it but
	// this test and the validator it drives.
	require.NoError(t, os.WriteFile(filepath.Join(protoDir, pkg+".proto"), []byte(body), 0o600))
}

func TestRun_ToolMissingField100(t *testing.T) {
	dir := componentDir(t, "demo-tool")
	writeMainGo(t, dir)
	writeToolProto(t, dir, "demotool", `syntax = "proto3";
package gibson.tools.demo.v1;
message DemoToolRequest { string target = 1; }
message DemoToolResponse { string raw = 1; }
`)

	r, err := validate.Run(dir, validate.KindTool)
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
	dir := componentDir(t, "demo-tool")
	writeMainGo(t, dir)
	writeToolProto(t, dir, "demotool", `syntax = "proto3";
package gibson.tools.demo.v1;
import "gibson/graphrag/v1/graphrag.proto";
message DemoToolRequest { string target = 1; }
message DemoToolResponse {
  string raw = 1;
  gibson.graphrag.v1.DiscoveryResult discovery = 100;
}
`)

	r, err := validate.Run(dir, validate.KindTool)
	require.NoError(t, err)
	// Allow buf-not-on-PATH to remain a warning; field 100 must not be an error.
	for _, e := range r.Errors {
		assert.NotContains(t, e.Message, "field 100", "field 100 must validate cleanly when present")
	}
}

// TestRun_ToolProtoPathComesFromTheDirectory is the link the conventions
// introduce: the proto path used to be derived from metadata.name in the file,
// and is now derived from the directory. A tool whose directory is named
// correctly must find its proto, and this fails if the two derivations diverge.
func TestRun_ToolProtoPathComesFromTheDirectory(t *testing.T) {
	dir := componentDir(t, "a-b-c")
	writeMainGo(t, dir)
	writeToolProto(t, dir, "abc", `syntax = "proto3";
package gibson.tools.abc.v1;
message Resp { gibson.graphrag.v1.DiscoveryResult discovery = 100; }
`)

	r, err := validate.Run(dir, validate.KindTool)
	require.NoError(t, err)
	for _, e := range r.Errors {
		assert.NotContains(t, e.Message, "tool proto not found",
			"the proto path must come from the directory name: %+v", r.Errors)
	}
}
