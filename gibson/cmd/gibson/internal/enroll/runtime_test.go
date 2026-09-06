// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package enroll_test

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/sdk/capabilitygrant"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/enroll"
)

func sampleCredential(t *testing.T) capabilitygrant.RuntimeCredential {
	t.Helper()
	key, err := capabilitygrant.GenerateAgentKey()
	require.NoError(t, err)
	return capabilitygrant.RuntimeCredential{
		HostID:         "host-1",
		AgentID:        "agent-9",
		ComponentScope: "component:hello",
		AgentKeySeed:   key.Seed(),
	}
}

func TestRuntimeInstall_SaveResolveRoundTrip(t *testing.T) {
	t.Setenv("GIBSON_HOME", t.TempDir())
	rc := sampleCredential(t)

	path, err := enroll.SaveRuntimeInstall("agent", "hello", enroll.RuntimeInstall{
		GibsonURL:  "https://daemon.example",
		Credential: rc,
	})
	require.NoError(t, err)
	assert.Contains(t, path, "agent/hello.runtime.json")

	gotRC, gotURL, err := enroll.ResolveRuntimeCredential("agent", "hello")
	require.NoError(t, err)
	assert.Equal(t, rc, gotRC)
	assert.Equal(t, "https://daemon.example", gotURL)
}

func TestResolveRuntimeCredential_MissingIsHardError(t *testing.T) {
	t.Setenv("GIBSON_HOME", t.TempDir())
	_, _, err := enroll.ResolveRuntimeCredential("agent", "absent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "register")
}

func TestResolveRuntimeCredential_EnvOverride(t *testing.T) {
	t.Setenv("GIBSON_HOME", t.TempDir()) // empty — env must win regardless
	rc := sampleCredential(t)
	enc, err := rc.Encode()
	require.NoError(t, err)
	t.Setenv(enroll.EnvRuntimeCredential, base64.StdEncoding.EncodeToString(enc))
	t.Setenv(enroll.EnvGibsonURL, "https://env.example")

	gotRC, gotURL, err := enroll.ResolveRuntimeCredential("agent", "")
	require.NoError(t, err)
	assert.Equal(t, rc, gotRC)
	assert.Equal(t, "https://env.example", gotURL)
}

func TestResolveRuntimeCredential_EnvWithoutURLFails(t *testing.T) {
	t.Setenv("GIBSON_HOME", t.TempDir())
	rc := sampleCredential(t)
	enc, _ := rc.Encode()
	t.Setenv(enroll.EnvRuntimeCredential, base64.StdEncoding.EncodeToString(enc))
	t.Setenv(enroll.EnvGibsonURL, "")

	_, _, err := enroll.ResolveRuntimeCredential("agent", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), enroll.EnvGibsonURL)
}

func TestListInstalls(t *testing.T) {
	t.Setenv("GIBSON_HOME", t.TempDir())
	rc := sampleCredential(t)
	_, err := enroll.SaveRuntimeInstall("agent", "a1", enroll.RuntimeInstall{GibsonURL: "u", Credential: rc})
	require.NoError(t, err)
	_, err = enroll.SaveRuntimeInstall("tool", "t1", enroll.RuntimeInstall{GibsonURL: "u", Credential: rc})
	require.NoError(t, err)

	installs, err := enroll.ListInstalls()
	require.NoError(t, err)
	require.Len(t, installs, 2)
	assert.Contains(t, installs, enroll.InstallRef{Kind: "agent", Name: "a1"})
	assert.Contains(t, installs, enroll.InstallRef{Kind: "tool", Name: "t1"})
}
