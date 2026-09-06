// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	"google.golang.org/grpc"
)

// writeTestSession writes a non-expired login session at
// ~/.gibson/auth/credentials (HOME redirected to a temp dir) pointing at
// the fake server over an http:// URL, so commands dial it through the
// normal authenticated session path (plaintext transport, no refresh).
// Shared by the submit and draft tests in this package.
func writeTestSession(t *testing.T, addr string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	creds := &deviceauth.Credentials{
		Issuer:       "http://127.0.0.1:1",
		ClientID:     "gibson-cli",
		TokenURL:     "http://127.0.0.1:1/token",
		AccessToken:  "test-access-token",
		Expiry:       time.Now().Add(time.Hour),
		ActiveTenant: "tenant-test",
		GibsonURL:    "http://" + addr,
	}
	require.NoError(t, creds.Save())
}

// fakeDaemonServer stubs only the two RPC methods used by submitCmd.
type fakeDaemonServer struct {
	daemonv1.UnimplementedDaemonServiceServer

	createDefFn func(context.Context, *daemonv1.CreateMissionDefinitionRequest) (*daemonv1.CreateMissionDefinitionResponse, error)
	createMsnFn func(context.Context, *daemonv1.CreateMissionRequest) (*daemonv1.CreateMissionResponse, error)
	runMsnFn    func(*daemonv1.RunMissionRequest, grpc.ServerStreamingServer[daemonv1.RunMissionResponse]) error
}

func (s *fakeDaemonServer) CreateMissionDefinition(ctx context.Context, req *daemonv1.CreateMissionDefinitionRequest) (*daemonv1.CreateMissionDefinitionResponse, error) {
	return s.createDefFn(ctx, req)
}

func (s *fakeDaemonServer) CreateMission(ctx context.Context, req *daemonv1.CreateMissionRequest) (*daemonv1.CreateMissionResponse, error) {
	return s.createMsnFn(ctx, req)
}

func (s *fakeDaemonServer) RunMission(req *daemonv1.RunMissionRequest, stream grpc.ServerStreamingServer[daemonv1.RunMissionResponse]) error {
	if s.runMsnFn == nil {
		return nil
	}
	return s.runMsnFn(req, stream)
}

// mustListen creates a TCP listener on a random port and returns it.
func mustListen(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return lis
}

// startFakeDaemonServer starts an in-process gRPC server and returns
// its address. Stopped via t.Cleanup.
func startFakeDaemonServer(t *testing.T, svc daemonv1.DaemonServiceServer) string {
	t.Helper()
	lis := mustListen(t)
	srv := grpc.NewServer()
	daemonv1.RegisterDaemonServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

func TestSubmitCmd_dryRun(t *testing.T) {
	// --dry-run must print JSON without making any gRPC call.
	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"dry","version":"1.0.0"}`), 0o644))

	cmd := submitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--dry-run", file})
	require.NoError(t, cmd.Execute())
	require.Contains(t, buf.String(), "dry")
}

func TestSubmitCmd_fullRound(t *testing.T) {
	// Full flow: CreateMissionDefinition → CreateMission → RunMission.
	//
	// A target is bound, because #176 made one mandatory: a mission defined with
	// no target could be created and then never run, so submit now refuses. The
	// test was left without one and went red on main (adk#179).
	const defID = "def-123"
	const msnID = "msn-456"
	const targetID = "9f1d0c4e-2c9a-4a6e-9a58-2c4b7f0c1a11"

	var ranMission string
	svc := &fakeDaemonServer{
		createDefFn: func(_ context.Context, req *daemonv1.CreateMissionDefinitionRequest) (*daemonv1.CreateMissionDefinitionResponse, error) {
			require.Equal(t, "roundtrip", req.GetDefinition().GetName())
			return &daemonv1.CreateMissionDefinitionResponse{MissionDefinitionId: defID}, nil
		},
		createMsnFn: func(_ context.Context, req *daemonv1.CreateMissionRequest) (*daemonv1.CreateMissionResponse, error) {
			require.Equal(t, defID, req.GetMissionDefinitionId())
			require.Equal(t, targetID, req.GetTargetId(), "the bound target must reach CreateMission")
			return &daemonv1.CreateMissionResponse{
				Mission: &daemonv1.Mission{Id: msnID},
			}, nil
		},
		runMsnFn: func(req *daemonv1.RunMissionRequest, _ grpc.ServerStreamingServer[daemonv1.RunMissionResponse]) error {
			ranMission = req.GetMissionDefinitionId()
			require.Equal(t, targetID, req.GetTargetId(), "the bound target must reach RunMission")
			return nil
		},
	}
	addr := startFakeDaemonServer(t, svc)
	writeTestSession(t, addr)

	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"roundtrip","version":"1.0.0"}`), 0o644))

	cmd := submitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--target", targetID, file})
	require.NoError(t, cmd.Execute())
	require.Contains(t, buf.String(), msnID)
	require.Equal(t, defID, ranMission, "submit must actually run the mission, not just define it")
}

// TestSubmitCmd_noTargetFailsWithGuidance is the other half of #176: a mission
// with no target could previously be defined and then never run, which looked
// like a successful submit. It must now fail, and say how to fix it.
func TestSubmitCmd_noTargetFailsWithGuidance(t *testing.T) {
	svc := &fakeDaemonServer{
		createDefFn: func(context.Context, *daemonv1.CreateMissionDefinitionRequest) (*daemonv1.CreateMissionDefinitionResponse, error) {
			t.Fatal("a mission with no target must be rejected before it is defined")
			return nil, nil
		},
	}
	addr := startFakeDaemonServer(t, svc)
	writeTestSession(t, addr)

	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"untargeted","version":"1.0.0"}`), 0o644))

	cmd := submitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{file})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "--target", "the error must say how to bind a target")
}

// TestSubmitCmd_requiresLogin verifies submit fails closed (with the "run
// gibson login" hint) when no session exists. The file is valid so
// execution reaches the dial.
func TestSubmitCmd_requiresLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"x","version":"1.0.0"}`), 0o644))

	cmd := submitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{file})
	err := cmd.Execute()
	require.ErrorIs(t, err, deviceauth.ErrNotLoggedIn)
}

// TestSubmitCmd_detach: --detach returns after the daemon's first frame with
// the mission id printed, while the run is still streaming. A caller that is
// itself a component of the mission (the interactive plugin) needs this: it
// cannot claim the dispatch while blocked on the stream.
func TestSubmitCmd_detach(t *testing.T) {
	const (
		defID    = "def-detach"
		msnID    = "msn-detach"
		targetID = "tgt-detach"
	)
	streamReleased := make(chan struct{})
	svc := &fakeDaemonServer{
		createDefFn: func(_ context.Context, _ *daemonv1.CreateMissionDefinitionRequest) (*daemonv1.CreateMissionDefinitionResponse, error) {
			return &daemonv1.CreateMissionDefinitionResponse{MissionDefinitionId: defID}, nil
		},
		createMsnFn: func(_ context.Context, _ *daemonv1.CreateMissionRequest) (*daemonv1.CreateMissionResponse, error) {
			return &daemonv1.CreateMissionResponse{Mission: &daemonv1.Mission{Id: msnID}}, nil
		},
		runMsnFn: func(_ *daemonv1.RunMissionRequest, stream grpc.ServerStreamingServer[daemonv1.RunMissionResponse]) error {
			if err := stream.Send(&daemonv1.RunMissionResponse{EventType: "status", MissionId: msnID}); err != nil {
				return err
			}
			// The run goes on; the client must not wait for it.
			<-stream.Context().Done()
			close(streamReleased)
			return nil
		},
	}
	addr := startFakeDaemonServer(t, svc)
	writeTestSession(t, addr)
	file := t.TempDir() + "/m.yaml"
	require.NoError(t, os.WriteFile(file, []byte(`{"name":"detach","version":"1.0.0"}`), 0o644))
	cmd := submitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--detach", "--target", targetID, file})
	require.NoError(t, cmd.Execute())
	require.Contains(t, buf.String(), msnID)
	select {
	case <-streamReleased:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream was not released after --detach returned")
	}
}
