// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/workspace"
)

func TestLoad_RejectsWorldWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.yaml")
	require.NoError(t, os.WriteFile(path, []byte("gibson_url: https://x\n"), 0o644))
	require.NoError(t, os.Chmod(path, 0o666)) // chmod after write to bypass umask
	_, err := workspace.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "world-writable")
}

func TestLoad_RejectsCredentialFields(t *testing.T) {
	cases := map[string]string{
		"client_id":       "gibson_url: https://x\nclient_id: foo\n",
		"client_secret":   "gibson_url: https://x\nclient_secret: foo\n",
		"bootstrap_token": "gibson_url: https://x\nbootstrap_token: foo\n",
		"host_key":        "gibson_url: https://x\nhost_key: foo\n",
		"password":        "gibson_url: https://x\npassword: foo\n",
		"secret":          "gibson_url: https://x\nsecret: foo\n",
		"token":           "gibson_url: https://x\ntoken: foo\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "workspace.yaml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			_, err := workspace.Load(path)
			require.Error(t, err)
			assert.ErrorIs(t, err, workspace.ErrCredentialField,
				"expected ErrCredentialField, got: %v", err)
		})
	}
}

func TestSave_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gibson", "workspace.yaml")
	in := &workspace.Workspace{
		GibsonURL: "https://example.zeroroot.ai",
		Comment:   "anthony's dev",
	}
	require.NoError(t, workspace.Save(path, in))

	out, err := workspace.Load(path)
	require.NoError(t, err)
	assert.Equal(t, in.GibsonURL, out.GibsonURL)
	assert.Equal(t, in.Comment, out.Comment)
}

func TestSave_RequiresGibsonURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.yaml")
	require.Error(t, workspace.Save(path, &workspace.Workspace{}))
}

func TestResolve_FlagWins(t *testing.T) {
	t.Setenv("GIBSON_URL", "https://from-env")
	res, err := workspace.Resolve("https://from-flag")
	require.NoError(t, err)
	assert.Equal(t, "flag", res.Source)
	assert.Equal(t, "https://from-flag", res.GibsonURL)
}

func TestResolve_EnvFallback(t *testing.T) {
	t.Setenv("GIBSON_URL", "https://from-env")

	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir())) // no local workspace
	defer os.Chdir(wd)                        //nolint:errcheck // restoring cwd in test cleanup; error not actionable

	res, err := workspace.Resolve("")
	require.NoError(t, err)
	assert.Equal(t, "env", res.Source)
	assert.Equal(t, "https://from-env", res.GibsonURL)
}

func TestResolve_LocalWorkspace(t *testing.T) {
	t.Setenv("GIBSON_URL", "")

	root := t.TempDir()
	require.NoError(t, workspace.Save(filepath.Join(root, ".gibson", "workspace.yaml"), &workspace.Workspace{
		GibsonURL: "https://from-local",
	}))

	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(root))
	defer os.Chdir(wd) //nolint:errcheck // restoring cwd in test cleanup; error not actionable

	res, err := workspace.Resolve("")
	require.NoError(t, err)
	assert.Equal(t, "local-workspace", res.Source)
	assert.Equal(t, "https://from-local", res.GibsonURL)
}

func TestResolve_NoSource(t *testing.T) {
	t.Setenv("GIBSON_URL", "")
	t.Setenv("HOME", t.TempDir()) // no global workspace

	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	defer os.Chdir(wd) //nolint:errcheck // restoring cwd in test cleanup; error not actionable

	_, err := workspace.Resolve("")
	require.Error(t, err)
	assert.ErrorIs(t, err, workspace.ErrNoGibsonURL)
}
