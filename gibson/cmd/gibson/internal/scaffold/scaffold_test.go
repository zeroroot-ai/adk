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
				{Name: "cred:api_key"},
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
				{Name: "cred:db_password"},
				{Name: "cred:token"},
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
			{Name: "cred:api_key"},
		},
	}

	files, err := scaffold.Render(input)
	require.NoError(t, err)

	wantFiles := []string{
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
	assert.NotContains(t, files, "plugin.yaml", "a plugin declares itself in code (ADR-0097)")
}

func TestRender_DefaultVersion(t *testing.T) {
	files, err := scaffold.Render(scaffold.ScaffoldInput{Name: "default-ver", Kind: scaffold.KindPlugin})
	require.NoError(t, err)
	assert.Contains(t, string(files["handler.go"]), `pluginVersion = "0.1.0"`)
}

// TestRender_PluginDeclaresItselfInCode: the handler names the plugin, its
// version and its method, and resolves each startup secret in OnStart, with
// no manifest file (ADR-0097).
func TestRender_PluginDeclaresItselfInCode(t *testing.T) {
	files, err := scaffold.Render(scaffold.ScaffoldInput{
		Name:    "smoketest",
		Version: "0.2.0",
		Kind:    scaffold.KindPlugin,
		Secrets: []scaffold.SecretInput{{Name: "cred:token"}},
	})
	require.NoError(t, err)
	handler := string(files["handler.go"])
	for _, want := range []string{
		`pluginName    = "smoketest"`,
		`pluginVersion = "0.2.0"`,
		"plugin.WithName(pluginName)",
		"plugin.WithVersion(pluginVersion)",
		`plugin.WithHandler("Echo", "Echo returns the request message unchanged.", echo)`,
		`"cred:token",`,
		"OnStart: requireSecrets",
	} {
		assert.Contains(t, handler, want)
	}
	assert.NotContains(t, handler, "WithManifest")

	plain, err := scaffold.Render(scaffold.ScaffoldInput{Name: "plain", Kind: scaffold.KindPlugin})
	require.NoError(t, err)
	assert.NotContains(t, string(plain["handler.go"]), "requireSecrets", "a plugin with no startup secret has no start check")
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
		"main.go", "go.mod",
		"Makefile", "Dockerfile", ".gitignore", "README.md",
		"ontology.yaml",
	} {
		assert.Contains(t, files, name, "agent scaffold missing %q", name)
	}
	// No proto, no buf config, no plugin.yaml.
	assert.NotContains(t, files, "plugin.yaml")
	assert.NotContains(t, files, "buf.yaml")
	// And no component.yaml: the scaffold stopped emitting it with ADR-0097
	// (adk#90). A component declares nothing on disk that the CLI then parses.
	assert.NotContains(t, files, "component.yaml")
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
		"main.go", "go.mod",
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
	assert.NotContains(t, files, "component.yaml", "the scaffold no longer emits component.yaml (adk#90)")

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
		Secrets: []scaffold.SecretInput{{Name: "cred:x"}},
	})
	require.Error(t, err)
}

func TestParseSecretFlag(t *testing.T) {
	tests := []struct {
		input   string
		want    scaffold.SecretInput
		wantErr bool
	}{
		{input: "cred:api_key", want: scaffold.SecretInput{Name: "cred:api_key"}},
		{input: " provider_config:openai ", want: scaffold.SecretInput{Name: "provider_config:openai"}},
		{input: "cred:api_key=startup:live", wantErr: true},
		{input: "no-prefix", wantErr: true},
		{input: "", wantErr: true},
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

// dockerfileSourcesNotScaffolded are the build-context files a scaffolded
// Dockerfile copies that Render does not emit, each with the reason another
// step produces it. An entry that no Dockerfile copies fails the test, so the
// list cannot outlive its reason.
var dockerfileSourcesNotScaffolded = map[string]string{
	"go.sum": "written by `go mod tidy`, which `gibson component build` runs on a fresh scaffold",
}

// TestRender_DockerfileCopiesOnlyFilesTheScaffoldEmits fails when a scaffolded
// Dockerfile copies a file out of the build context that the same scaffold
// does not write. The agent and tool Dockerfiles copied component.yaml after
// the scaffold stopped emitting it (adk#90), so `docker build` failed on a
// freshly scaffolded component.
func TestRender_DockerfileCopiesOnlyFilesTheScaffoldEmits(t *testing.T) {
	exemptSeen := map[string]bool{}
	dockerfiles := 0
	for _, kind := range []scaffold.Kind{scaffold.KindAgent, scaffold.KindTool, scaffold.KindPlugin} {
		files, err := scaffold.Render(scaffold.ScaffoldInput{
			Name:       "demo",
			Version:    "0.1.0",
			Kind:       kind,
			SDKVersion: "v1.2.0",
		})
		require.NoError(t, err)
		dockerfile, ok := files["Dockerfile"]
		require.True(t, ok, "%s scaffold emits no Dockerfile", kind)
		dockerfiles++

		for line := range strings.SplitSeq(string(dockerfile), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 || fields[0] != "COPY" || strings.HasPrefix(fields[1], "--from=") {
				continue
			}
			for _, src := range fields[1 : len(fields)-1] {
				src = strings.TrimSuffix(src, "*")
				if src == "." {
					continue
				}
				if _, exempt := dockerfileSourcesNotScaffolded[src]; exempt {
					exemptSeen[src] = true
					continue
				}
				if _, emitted := files[src]; !emitted {
					t.Errorf("the %s Dockerfile copies %q, which the %s scaffold does not emit", kind, src, kind)
				}
			}
		}
	}
	require.Equal(t, 3, dockerfiles)
	for src, reason := range dockerfileSourcesNotScaffolded {
		assert.True(t, exemptSeen[src], "no scaffolded Dockerfile copies %q any more; delete its exemption (%s)", src, reason)
	}
}
