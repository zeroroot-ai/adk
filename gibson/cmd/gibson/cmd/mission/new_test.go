// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	targetv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/target/v1"
)

// templateNames is every shipped template, sorted, so a template added
// without a target placeholder fails the suite rather than a newcomer's
// first submit.
func templateNames() []string {
	names := make([]string, 0, len(builtinTemplates))
	for n := range builtinTemplates {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func TestEveryTemplateCarriesExactlyOneTargetPlaceholder(t *testing.T) {
	// The scaffolder fills the target by rewriting this one line. A template
	// with none would scaffold un-submittable output; one with two would
	// leave a placeholder behind.
	for _, name := range templateNames() {
		t.Run(name, func(t *testing.T) {
			n := targetRefRx.FindAllStringIndex(builtinTemplates[name].Body, -1)
			require.Len(t, n, 1, "template %q must carry exactly one target_ref line", name)
		})
	}
	require.Len(t, targetRefRx.FindAllStringIndex(minimalScaffold, -1), 1,
		"the minimal scaffold must carry exactly one target_ref line")
}

// listOnlyDaemon serves ListTargets and nothing else.
type listOnlyDaemon struct {
	daemonv1.UnimplementedDaemonServiceServer
	targets []*targetv1.Target
	calls   int
}

func (s *listOnlyDaemon) ListTargets(_ context.Context, _ *daemonv1.ListTargetsRequest) (*daemonv1.ListTargetsResponse, error) {
	s.calls++
	return &daemonv1.ListTargetsResponse{Targets: s.targets}, nil
}

func target(id, name string) *targetv1.Target {
	return &targetv1.Target{Id: id, Name: name, Type: "domain", Status: "active"}
}

func runNew(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := newCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// The issue's acceptance test: every shipped template scaffolds output that
// submits against a fake daemon with no extra flag. Before the fix each one
// carried target_ref: "", and submit refused with "no target".
func TestNewCmd_everyTemplateScaffoldsASubmittableMission(t *testing.T) {
	only := uuid.NewString()
	srv := &listOnlyDaemon{targets: []*targetv1.Target{target(only, "prod-web")}}
	addr := startFakeDaemonServer(t, srv)

	for _, name := range templateNames() {
		t.Run(name, func(t *testing.T) {
			writeTestSession(t, addr)
			file := filepath.Join(t.TempDir(), "mission.cue")

			_, stderr, err := runNew(t, "--from-template", name, "-o", file)
			require.NoError(t, err)
			require.Contains(t, stderr, "prod-web", "the scaffolder names the target it chose")

			body, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Contains(t, string(body), only, "the resolved target UUID is written into the scaffold")
			require.NotContains(t, string(body), `target_ref:  ""`, "no empty placeholder survives")

			// The scaffold loads, validates, and carries the target submit needs.
			def, err := loadMissionFile(file, "")
			require.NoError(t, err)
			require.Equal(t, only, def.GetTargetRef())
			require.NoError(t, checkTargetRef(def.GetTargetRef()))
		})
	}
}

func TestNewCmd_targetByName(t *testing.T) {
	webID, dbID := uuid.NewString(), uuid.NewString()
	srv := &listOnlyDaemon{targets: []*targetv1.Target{target(webID, "prod-web"), target(dbID, "prod-db")}}
	addr := startFakeDaemonServer(t, srv)
	writeTestSession(t, addr)

	out, _, err := runNew(t, "--from-template", "recon", "--target", "prod-db")
	require.NoError(t, err)
	require.Contains(t, out, dbID)
	require.NotContains(t, out, webID)
}

func TestNewCmd_targetByUUIDSkipsTheDaemon(t *testing.T) {
	// A UUID needs no lookup, so --target <uuid> works with no session.
	t.Setenv("HOME", t.TempDir())
	id := uuid.NewString()

	out, _, err := runNew(t, "--from-template", "recon", "--target", id)
	require.NoError(t, err)
	require.Contains(t, out, id)
}

func TestNewCmd_severalTargetsRefusesAndListsThem(t *testing.T) {
	srv := &listOnlyDaemon{targets: []*targetv1.Target{
		target(uuid.NewString(), "prod-web"),
		target(uuid.NewString(), "prod-db"),
	}}
	addr := startFakeDaemonServer(t, srv)
	writeTestSession(t, addr)

	_, _, err := runNew(t, "--from-template", "recon")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--target")
	require.Contains(t, err.Error(), "prod-web")
	require.Contains(t, err.Error(), "prod-db")
}

func TestNewCmd_noTargetsNamesTheCreateCommand(t *testing.T) {
	srv := &listOnlyDaemon{}
	addr := startFakeDaemonServer(t, srv)
	writeTestSession(t, addr)

	_, _, err := runNew(t, "--from-template", "recon")
	require.Error(t, err)
	require.Contains(t, err.Error(), "gibson target create")
}

func TestNewCmd_unknownTargetNameRefuses(t *testing.T) {
	srv := &listOnlyDaemon{targets: []*targetv1.Target{target(uuid.NewString(), "prod-web")}}
	addr := startFakeDaemonServer(t, srv)
	writeTestSession(t, addr)

	_, _, err := runNew(t, "--from-template", "recon", "--target", "staging")
	require.Error(t, err)
	require.Contains(t, err.Error(), "staging")
	require.Contains(t, err.Error(), "prod-web")
}

func TestNewCmd_noTargetFlagScaffoldsOfflineWithThePlaceholder(t *testing.T) {
	// Offline scaffolding stays available, but it is asked for rather than
	// being what you get by accident.
	t.Setenv("HOME", t.TempDir())

	out, stderr, err := runNew(t, "--from-template", "recon", "--no-target")
	require.NoError(t, err)
	require.Contains(t, out, `target_ref:  ""`)
	require.Contains(t, stderr, "--target")
}

func TestNewCmd_listTemplatesNeedsNoSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	out, _, err := runNew(t, "--list-templates")
	require.NoError(t, err)
	for _, name := range templateNames() {
		require.Contains(t, out, name)
	}
}

func TestNewCmd_loggedOutSaysSo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, _, err := runNew(t, "--from-template", "recon")
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "gibson login") || strings.Contains(err.Error(), "--no-target"),
		"a logged-out scaffold must name the way forward, got: %v", err)
}
