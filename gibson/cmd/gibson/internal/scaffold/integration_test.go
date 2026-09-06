// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

//go:build integration

// Copyright (c) 2026 ZeroRoot
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scaffold_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/scaffold"
)

// TestIntegration_AgentBuilds renders the agent scaffold to a temp
// dir, runs `go mod tidy` + `go build ./...`, and asserts the binary
// builds. Requires network access to the Go module proxy.
//
// Run with: go test -tags=integration ./cmd/gibson/internal/scaffold
func TestIntegration_AgentBuilds(t *testing.T) {
	dir := renderTo(t, scaffold.ScaffoldInput{
		Name:       "demo-agent",
		Version:    "0.1.0",
		Kind:       scaffold.KindAgent,
		SDKVersion: "v1.2.0",
	})

	runCmd(t, dir, "go", "mod", "tidy")
	runCmd(t, dir, "go", "build", "./...")
}

// TestIntegration_ToolBuildsWithProto renders the tool scaffold,
// runs `make proto && go build`, and asserts the generated .pb.go
// declares Discovery as a *graphragpb.DiscoveryResult on field 100.
// Requires buf, protoc-gen-go, protoc-gen-go-grpc on PATH.
func TestIntegration_ToolBuildsWithProto(t *testing.T) {
	if _, err := exec.LookPath("buf"); err != nil {
		t.Skip("buf not on PATH; skipping tool integration test")
	}

	dir := renderTo(t, scaffold.ScaffoldInput{
		Name:       "demo-tool",
		Version:    "0.1.0",
		Kind:       scaffold.KindTool,
		SDKVersion: "v1.2.0",
	})

	runCmd(t, dir, "go", "mod", "tidy")
	runCmd(t, dir, "buf", "generate")
	runCmd(t, dir, "go", "build", "./...")

	pbPath := filepath.Join(dir, "api", "gen", "gibson", "tools", "demotool", "v1", "demotool.pb.go")
	b, err := os.ReadFile(pbPath)
	require.NoError(t, err, "expected generated %s", pbPath)
	pb := string(b)
	// The generated struct must reference DiscoveryResult and proto tag
	// 100 in the response message.
	require.True(t, strings.Contains(pb, "graphragv1.DiscoveryResult") || strings.Contains(pb, "DiscoveryResult"),
		"%s must reference DiscoveryResult", pbPath)
	require.True(t, strings.Contains(pb, "protobuf:\"bytes,100,") || strings.Contains(pb, ",100,"),
		"%s must contain proto tag 100", pbPath)
}

// TestIntegration_PluginBuildsAndTests renders the Go-first plugin scaffold
// (ADR-0065 R4 — no proto, no buf), resolves modules, builds the binary, and
// runs the committed cassette test. Requires network access to the Go module
// proxy for the SDK.
func TestIntegration_PluginBuildsAndTests(t *testing.T) {
	dir := renderTo(t, scaffold.ScaffoldInput{
		Name:       "demo-plugin",
		Version:    "0.1.0",
		Kind:       scaffold.KindPlugin,
		SDKVersion: "v0.168.0",
	})

	runCmd(t, dir, "go", "mod", "tidy")
	runCmd(t, dir, "go", "build", "./...")
	runCmd(t, dir, "go", "test", "./...")
}

// TestIntegration_ConnectorValidates renders the declarative connector scaffold
// (ADR-0065 R6) and runs its schema-validation smoke test. Requires network
// access to the Go module proxy for gopkg.in/yaml.v3.
func TestIntegration_ConnectorValidates(t *testing.T) {
	dir := renderTo(t, scaffold.ScaffoldInput{
		Name:    "demo-connector",
		Version: "0.1.0",
		Kind:    scaffold.KindConnector,
	})

	runCmd(t, dir, "go", "mod", "tidy")
	runCmd(t, dir, "go", "test", "./...")
}

// renderTo renders input into a t.TempDir and writes every file to disk.
func renderTo(t *testing.T, input scaffold.ScaffoldInput) string {
	t.Helper()
	files, err := scaffold.Render(input)
	require.NoError(t, err)

	dir := t.TempDir()
	for rel, content := range files {
		dst := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		require.NoError(t, os.WriteFile(dst, content, 0o644))
	}
	return dir
}

// runCmd runs a command in dir; logs combined output and fails the test
// on non-zero exit.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s in %s failed: %v\noutput:\n%s",
			name, strings.Join(args, " "), dir, err, buf.String())
	}
}
