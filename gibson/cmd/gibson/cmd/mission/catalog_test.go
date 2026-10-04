// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	missionv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/mission/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// parseParams is what turns --param flags into the map the daemon validates.
// Every case here is a caller mistake that must not reach the daemon as a
// silently different request.
func TestParseParams(t *testing.T) {
	t.Parallel()

	got, err := parseParams([]string{
		"kubeconfigSecret=cred:goat-cluster",
		"bank=bank/core-banking",
	})
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if got["kubeconfigSecret"] != "cred:goat-cluster" || got["bank"] != "bank/core-banking" {
		t.Errorf("parseParams = %v", got)
	}
}

// A value may contain '=' — a URL query, a base64 pad — so only the FIRST
// separator splits. Cutting at the last one would truncate the value and the
// daemon would accept the truncation without complaint.
func TestParseParams_SplitsOnTheFirstSeparatorOnly(t *testing.T) {
	t.Parallel()

	got, err := parseParams([]string{"manifestsProject=group/repo?ref=main"})
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if want := "group/repo?ref=main"; got["manifestsProject"] != want {
		t.Errorf("value = %q, want %q", got["manifestsProject"], want)
	}
}

// An empty value is a value. A mission's parameter set is closed and every name
// is required, so the daemon refuses a blank one — which is a better place for
// that decision than here, where it would read as "the flag did not work".
func TestParseParams_AnEmptyValueIsPassedThrough(t *testing.T) {
	t.Parallel()

	got, err := parseParams([]string{"bank="})
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if v, ok := got["bank"]; !ok || v != "" {
		t.Errorf("parseParams = %v, want bank present and empty", got)
	}
}

// A duplicate is an error, not last-wins. Two values for one parameter is a
// caller that does not know which it sent, and keeping one silently is how a
// run ends up pointed somewhere nobody chose.
func TestParseParams_ADuplicateKeyIsRefused(t *testing.T) {
	t.Parallel()

	_, err := parseParams([]string{"bank=a", "bank=b"})
	if err == nil {
		t.Fatal("a duplicate --param was accepted")
	}
	if !strings.Contains(err.Error(), "bank") {
		t.Errorf("the error does not name the duplicated key: %v", err)
	}
}

func TestParseParams_RefusesWhatIsNotKeyValue(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"bank", "", "=novalue", "  =x"} {
		if _, err := parseParams([]string{bad}); err == nil {
			t.Errorf("--param %q was accepted", bad)
		}
	}
}

func TestParseParams_NoneIsAnEmptyMapNotAnError(t *testing.T) {
	t.Parallel()

	got, err := parseParams(nil)
	if err != nil {
		t.Fatalf("parseParams(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parseParams(nil) = %v, want empty", got)
	}
}

// ── the catalog path, against a fake daemon ─────────────────────────────────

// fakeCatalogServer answers the two catalog reads and the three calls a submit
// makes after them, and records what it was asked.
type fakeCatalogServer struct {
	daemonv1.UnimplementedDaemonServiceServer

	listResp *daemonv1.ListCatalogMissionsResponse

	renderedName   string
	renderedParams map[string]string
	renderResp     *daemonv1.RenderCatalogMissionResponse
	renderErr      error

	createdDefName string
	createdTarget  string
}

func (s *fakeCatalogServer) ListCatalogMissions(_ context.Context, _ *daemonv1.ListCatalogMissionsRequest) (*daemonv1.ListCatalogMissionsResponse, error) {
	return s.listResp, nil
}

func (s *fakeCatalogServer) RenderCatalogMission(_ context.Context, req *daemonv1.RenderCatalogMissionRequest) (*daemonv1.RenderCatalogMissionResponse, error) {
	s.renderedName = req.GetName()
	s.renderedParams = req.GetParams()
	if s.renderErr != nil {
		return nil, s.renderErr
	}
	return s.renderResp, nil
}

func (s *fakeCatalogServer) CreateMissionDefinition(_ context.Context, req *daemonv1.CreateMissionDefinitionRequest) (*daemonv1.CreateMissionDefinitionResponse, error) {
	s.createdDefName = req.GetDefinition().GetName()
	return &daemonv1.CreateMissionDefinitionResponse{MissionDefinitionId: "def-1"}, nil
}

func (s *fakeCatalogServer) CreateMission(_ context.Context, req *daemonv1.CreateMissionRequest) (*daemonv1.CreateMissionResponse, error) {
	s.createdTarget = req.GetTargetId()
	return &daemonv1.CreateMissionResponse{
		Success: true,
		Mission: &daemonv1.Mission{Id: "mission-1"},
	}, nil
}

func (s *fakeCatalogServer) RunMission(_ *daemonv1.RunMissionRequest, _ grpc.ServerStreamingServer[daemonv1.RunMissionResponse]) error {
	return nil
}

func clusterAssessmentFake() *fakeCatalogServer {
	return &fakeCatalogServer{
		listResp: &daemonv1.ListCatalogMissionsResponse{
			Missions: []*daemonv1.CatalogMission{{
				Name:           "cluster-assessment",
				Description:    "Assess one Kubernetes cluster.",
				Version:        "1.0.0",
				DeclaredParams: []string{"bank", "forgeConnector", "kubeconfigSecret", "manifestsProject"},
			}},
		},
		renderResp: &daemonv1.RenderCatalogMissionResponse{
			Mission: &missionv1.MissionDefinition{Name: "cluster-assessment", Version: "1.0.0"},
			Source:  "mission: {name: \"cluster-assessment\"}",
		},
	}
}

// The whole point of gibson#631: a person names a shipped mission and it runs,
// with no file on disk and no copy of the graph.
func TestSubmitCmd_catalogMissionRunsWithoutAFile(t *testing.T) {
	fake := clusterAssessmentFake()
	addr := startFakeDaemonServer(t, fake)
	writeTestSession(t, addr)

	cmd := submitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--catalog", "cluster-assessment",
		"--target", "11111111-1111-1111-1111-111111111111",
		"--param", "kubeconfigSecret=cred:goat-cluster",
		"--param", "bank=bank/core-banking",
	})
	require.NoError(t, cmd.Execute())

	require.Equal(t, "cluster-assessment", fake.renderedName)
	require.Equal(t, "cred:goat-cluster", fake.renderedParams["kubeconfigSecret"])
	require.Equal(t, "bank/core-banking", fake.renderedParams["bank"])
	// The definition that was registered is the one the DAEMON rendered, not
	// anything this command built.
	require.Equal(t, "cluster-assessment", fake.createdDefName)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", fake.createdTarget)
	require.Contains(t, out.String(), "mission-1")
}

// A catalog mission declares no target, so --target is the caller's to supply.
// Without it the command must say so before it creates anything server-side.
func TestSubmitCmd_catalogMissionNeedsATarget(t *testing.T) {
	fake := clusterAssessmentFake()
	addr := startFakeDaemonServer(t, fake)
	writeTestSession(t, addr)

	cmd := submitCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--catalog", "cluster-assessment", "--param", "bank=b"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no target")
	// Nothing was registered: a local requirement must not cost a server-side
	// write.
	require.Empty(t, fake.createdDefName)
}

// A file and --catalog are two answers to "what runs". Said here rather than
// three calls later, and before anything is dialed.
func TestSubmitCmd_aFileAndCatalogTogetherIsRefused(t *testing.T) {
	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"m","version":"1.0.0"}`), 0o600))

	cmd := submitCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{file, "--catalog", "cluster-assessment"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not both")
}

func TestSubmitCmd_neitherAFileNorCatalogIsRefused(t *testing.T) {
	cmd := submitCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "--catalog")
}

// --param with a file would otherwise be dropped, and the caller would believe
// their parameters had bound to a mission that has none.
func TestSubmitCmd_paramWithAFileIsRefused(t *testing.T) {
	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"m","version":"1.0.0"}`), 0o600))

	cmd := submitCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{file, "--param", "k=v"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "--param")
}

// The daemon's refusal reaches the person unchanged. It is the side that knows
// the closed parameter set, and a CLI that reworded it would hide which key was
// wrong.
func TestSubmitCmd_catalogRenderRefusalReachesTheCaller(t *testing.T) {
	fake := clusterAssessmentFake()
	fake.renderErr = status.Error(codes.InvalidArgument,
		`missioncatalog: mission "cluster-assessment" has no parameter host (it takes bank, forgeConnector, kubeconfigSecret, manifestsProject)`)
	addr := startFakeDaemonServer(t, fake)
	writeTestSession(t, addr)

	cmd := submitCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--catalog", "cluster-assessment",
		"--target", "11111111-1111-1111-1111-111111111111",
		"--param", "host=evil.example.com",
	})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no parameter host")
	require.Empty(t, fake.createdDefName)
}

// The listing names every parameter, because a caller builds its --param flags
// from it.
func TestCatalogList_NamesEveryParameter(t *testing.T) {
	addr := startFakeDaemonServer(t, clusterAssessmentFake())
	writeTestSession(t, addr)

	cmd := catalogCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())

	got := out.String()
	require.Contains(t, got, "cluster-assessment")
	require.Contains(t, got, "Assess one Kubernetes cluster.")
	for _, p := range []string{"bank", "forgeConnector", "kubeconfigSecret", "manifestsProject"} {
		require.Contains(t, got, p)
	}
}

// An empty catalog says so. Printed as nothing it would read as a broken
// command rather than as an empty catalog.
func TestCatalogList_AnEmptyCatalogSaysSo(t *testing.T) {
	addr := startFakeDaemonServer(t, &fakeCatalogServer{
		listResp: &daemonv1.ListCatalogMissionsResponse{},
	})
	writeTestSession(t, addr)

	cmd := catalogCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "ships no missions")
}

// show prints the CUE verbatim, so what the daemon will run can be read rather
// than trusted.
func TestCatalogShow_PrintsTheSourceVerbatim(t *testing.T) {
	fake := clusterAssessmentFake()
	addr := startFakeDaemonServer(t, fake)
	writeTestSession(t, addr)

	cmd := catalogCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"show", "cluster-assessment"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), `mission: {name: "cluster-assessment"}`)
	require.Equal(t, "cluster-assessment", fake.renderedName)
}

func TestCatalogShow_RenderedPrintsTheDefinition(t *testing.T) {
	addr := startFakeDaemonServer(t, clusterAssessmentFake())
	writeTestSession(t, addr)

	cmd := catalogCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"show", "cluster-assessment", "--rendered",
		"--param", "bank=b", "--param", "kubeconfigSecret=cred:x",
		"--param", "forgeConnector=f", "--param", "manifestsProject=g/r"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), `"name"`)
	require.Contains(t, out.String(), "cluster-assessment")
}
