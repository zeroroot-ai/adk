// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package scaffold_test

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/sdk/plugin/manifest"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/scaffold"
)

// updateGoldens, when -update is passed, rewrites the golden files in
// testdata/golden/ from the current Render output. Use this after an
// intentional template change. Without -update, the tests fail on any
// drift.
//
//	go test ./cmd/gibson/internal/scaffold -update
var updateGoldens = flag.Bool("update", false, "regenerate golden files from current Render output")

// goldenCase pins a deterministic ScaffoldInput to a directory under
// testdata/golden/<kind>/<name>/. Adding a case requires running the
// test once with -update to seed the goldens.
type goldenCase struct {
	dir   string
	input scaffold.ScaffoldInput
}

var pluginGoldenCases = []goldenCase{
	{
		dir: "plugin/minimal",
		input: scaffold.ScaffoldInput{
			Name:    "byte-identity",
			Version: "0.1.0",
			Kind:    scaffold.KindPlugin,
		},
	},
	{
		dir: "plugin/with-one-secret",
		input: scaffold.ScaffoldInput{
			Name:    "byte-identity-secret",
			Version: "1.2.3",
			Kind:    scaffold.KindPlugin,
			Secrets: []scaffold.SecretInput{
				{Name: "cred:api_key", Scope: "startup", Rotation: "live"},
			},
		},
	},
	{
		dir: "plugin/with-multiple-secrets",
		input: scaffold.ScaffoldInput{
			Name:    "byte-identity-multi",
			Version: "0.99.0",
			Kind:    scaffold.KindPlugin,
			Secrets: []scaffold.SecretInput{
				{Name: "cred:db_password", Scope: "startup", Rotation: "live"},
				{Name: "cred:token", Scope: "per_call", Rotation: "restart"},
			},
		},
	},
	{
		dir: "agent/minimal",
		input: scaffold.ScaffoldInput{
			Name:       "demo-agent",
			Version:    "0.1.0",
			Kind:       scaffold.KindAgent,
			SDKVersion: "v1.2.0",
		},
	},
	{
		dir: "tool/minimal",
		input: scaffold.ScaffoldInput{
			Name:       "demo-tool",
			Version:    "0.1.0",
			Kind:       scaffold.KindTool,
			SDKVersion: "v1.2.0",
		},
	},
	{
		dir: "connector/minimal",
		input: scaffold.ScaffoldInput{
			Name:       "demo-connector",
			Version:    "0.1.0",
			Kind:       scaffold.KindConnector,
			SDKVersion: "v1.2.0",
		},
	},
}

func TestRender_AllFilesPresent(t *testing.T) {
	input := scaffold.ScaffoldInput{
		Name:    "my-plugin",
		Version: "0.1.0",
		Kind:    scaffold.KindPlugin,
		Secrets: []scaffold.SecretInput{
			{Name: "cred:api_key", Scope: "startup", Rotation: "live"},
		},
	}

	files, err := scaffold.Render(input)
	require.NoError(t, err)

	wantFiles := []string{
		"component.yaml",
		"plugin.yaml",
		"go.mod",
		"handler.go",
		"handler_test.go",
		"testdata/echo.json",
		"helm/values.yaml",
		"Makefile",
		"Dockerfile",
		".gitignore",
		"README.md",
	}
	for _, name := range wantFiles {
		assert.Contains(t, files, name, "expected output file %q to be present", name)
	}
}

func TestRender_DefaultVersion(t *testing.T) {
	files, err := scaffold.Render(scaffold.ScaffoldInput{Name: "default-ver", Kind: scaffold.KindPlugin})
	require.NoError(t, err)
	assert.Contains(t, string(files["plugin.yaml"]), "version: 0.1.0")
}

func TestRender_ManifestValidates(t *testing.T) {
	input := scaffold.ScaffoldInput{
		Name:    "smoketest",
		Version: "0.2.0",
		Kind:    scaffold.KindPlugin,
		Secrets: []scaffold.SecretInput{
			{Name: "cred:token", Scope: "per_call", Rotation: "restart"},
		},
	}

	files, err := scaffold.Render(input)
	require.NoError(t, err)

	manifestBytes := files["plugin.yaml"]
	require.NotEmpty(t, manifestBytes)

	m, err := manifest.LoadBytes(manifestBytes)
	require.NoError(t, err, "rendered plugin.yaml must validate against the manifest schema")
	assert.Equal(t, "smoketest", m.Metadata.Name)
	assert.Equal(t, "0.2.0", m.Metadata.Version)
	require.Len(t, m.Spec.Secrets, 1)
	assert.Equal(t, "cred:token", m.Spec.Secrets[0].Name)
}

// TestRender_PluginGoldenFiles is the migration's no-regression contract
// after the SDK scaffold package was removed in v1.2.0. For each pinned
// input in pluginGoldenCases, current Render output must match the
// committed bytes under testdata/golden/<dir>/. Drift fails the test.
//
// Run with -update to regenerate goldens after an intentional template
// change. Add new cases by appending to pluginGoldenCases and running
// once with -update.
func TestRender_PluginGoldenFiles(t *testing.T) {
	for _, tc := range pluginGoldenCases {
		t.Run(tc.dir, func(t *testing.T) {
			files, err := scaffold.Render(tc.input)
			require.NoError(t, err)

			goldenDir := filepath.Join("testdata", "golden", tc.dir)

			if *updateGoldens {
				require.NoError(t, os.RemoveAll(goldenDir))
				require.NoError(t, os.MkdirAll(goldenDir, 0o755))
				for name, content := range files {
					path := filepath.Join(goldenDir, name)
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
					require.NoError(t, os.WriteFile(path, content, 0o644))
				}
				t.Logf("regenerated %d golden files under %s", len(files), goldenDir)
				return
			}

			golden := loadGoldenDir(t, goldenDir)

			gotKeys, wantKeys := keysOf(files), keysOf(golden)
			sort.Strings(gotKeys)
			sort.Strings(wantKeys)
			require.Equal(t, wantKeys, gotKeys, "rendered file set must match golden directory exactly")

			for name, want := range golden {
				assert.Equal(t, string(want), string(files[name]),
					"file %q drifted from golden — re-run with -update if intentional", name)
			}
		})
	}
}

func TestRender_AgentAllFilesPresent(t *testing.T) {
	files, err := scaffold.Render(scaffold.ScaffoldInput{
		Name:       "demo-agent",
		Version:    "0.1.0",
		Kind:       scaffold.KindAgent,
		SDKVersion: "v1.2.0",
	})
	require.NoError(t, err)

	for _, name := range []string{
		"component.yaml", "main.go", "go.mod",
		"Makefile", "Dockerfile", ".gitignore", "README.md",
		"ontology.yaml",
	} {
		assert.Contains(t, files, name, "agent scaffold missing %q", name)
	}
	// No proto, no buf config, no plugin.yaml.
	assert.NotContains(t, files, "plugin.yaml")
	assert.NotContains(t, files, "buf.yaml")
}

func TestRender_ToolAllFilesPresent(t *testing.T) {
	files, err := scaffold.Render(scaffold.ScaffoldInput{
		Name:       "demo-tool",
		Version:    "0.1.0",
		Kind:       scaffold.KindTool,
		SDKVersion: "v1.2.0",
	})
	require.NoError(t, err)

	wantFiles := []string{
		"component.yaml", "main.go", "go.mod",
		"Makefile", "Dockerfile", ".gitignore", "README.md",
		"buf.yaml", "buf.gen.yaml",
		"ontology.yaml",
		"api/proto/gibson/tools/demotool/v1/demotool.proto",
		"proto/vendor/gibson/graphrag/v1/graphrag.proto",
		"proto/vendor/taxonomy/v1/taxonomy.proto",
	}
	for _, name := range wantFiles {
		assert.Contains(t, files, name, "tool scaffold missing %q", name)
	}
	// Field-100 contract is encoded in the proto template.
	assert.Contains(t, string(files["api/proto/gibson/tools/demotool/v1/demotool.proto"]),
		"gibson.graphrag.v1.DiscoveryResult discovery = 100",
		"tool proto must reserve field 100 for DiscoveryResult")
}

func TestRender_RejectsInvalidKind(t *testing.T) {
	_, err := scaffold.Render(scaffold.ScaffoldInput{Name: "x", Kind: scaffold.Kind("nonsense")})
	require.Error(t, err)
}

func TestRender_RejectsSecretsForNonPlugin(t *testing.T) {
	_, err := scaffold.Render(scaffold.ScaffoldInput{
		Name:    "x",
		Kind:    scaffold.KindAgent,
		Secrets: []scaffold.SecretInput{{Name: "cred:x", Scope: "startup", Rotation: "live"}},
	})
	require.Error(t, err)
}

func TestParseSecretFlag(t *testing.T) {
	tests := []struct {
		input   string
		want    scaffold.SecretInput
		wantErr bool
	}{
		{
			input: "cred:api_key=startup:live",
			want:  scaffold.SecretInput{Name: "cred:api_key", Scope: "startup", Rotation: "live"},
		},
		{
			input: "cred:db_password=per_call:restart",
			want:  scaffold.SecretInput{Name: "cred:db_password", Scope: "per_call", Rotation: "restart"},
		},
		{input: "no-equals", wantErr: true},
		{input: "name=badscope:live", wantErr: true},
		{input: "name=startup:badrotation", wantErr: true},
		{input: "name=startup", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := scaffold.ParseSecretFlag(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// loadGoldenDir reads every regular file (including dotfiles) in dir and
// returns a name→content map keyed by filename relative to dir.
func loadGoldenDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		// Normalise path separators for cross-platform stability.
		rel = strings.ReplaceAll(rel, string(filepath.Separator), "/")
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = b
		return nil
	})
	require.NoError(t, err, "load golden dir %s — run `go test -update` to seed goldens", dir)
	require.NotEmpty(t, out, "golden dir %s is empty — run `go test -update` to seed goldens", dir)
	return out
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
