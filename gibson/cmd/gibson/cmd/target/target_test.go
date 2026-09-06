// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package target

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	targetv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/target/v1"
	"google.golang.org/grpc"
)

// fakeDaemonServer stubs only the target RPCs on DaemonService; every
// other method is left unimplemented (forward-compatible via the embed).
type fakeDaemonServer struct {
	daemonv1.UnimplementedDaemonServiceServer

	createFn func(context.Context, *daemonv1.CreateTargetRequest) (*daemonv1.CreateTargetResponse, error)
	listFn   func(context.Context, *daemonv1.ListTargetsRequest) (*daemonv1.ListTargetsResponse, error)
	getFn    func(context.Context, *daemonv1.GetTargetRequest) (*daemonv1.GetTargetResponse, error)
	updateFn func(context.Context, *daemonv1.UpdateTargetRequest) (*daemonv1.UpdateTargetResponse, error)
	deleteFn func(context.Context, *daemonv1.DeleteTargetRequest) (*daemonv1.DeleteTargetResponse, error)
}

func (s *fakeDaemonServer) CreateTarget(ctx context.Context, req *daemonv1.CreateTargetRequest) (*daemonv1.CreateTargetResponse, error) {
	return s.createFn(ctx, req)
}

func (s *fakeDaemonServer) ListTargets(ctx context.Context, req *daemonv1.ListTargetsRequest) (*daemonv1.ListTargetsResponse, error) {
	return s.listFn(ctx, req)
}

func (s *fakeDaemonServer) GetTarget(ctx context.Context, req *daemonv1.GetTargetRequest) (*daemonv1.GetTargetResponse, error) {
	return s.getFn(ctx, req)
}

func (s *fakeDaemonServer) UpdateTarget(ctx context.Context, req *daemonv1.UpdateTargetRequest) (*daemonv1.UpdateTargetResponse, error) {
	return s.updateFn(ctx, req)
}

func (s *fakeDaemonServer) DeleteTarget(ctx context.Context, req *daemonv1.DeleteTargetRequest) (*daemonv1.DeleteTargetResponse, error) {
	return s.deleteFn(ctx, req)
}

func startFakeDaemonServer(t *testing.T, svc daemonv1.DaemonServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	daemonv1.RegisterDaemonServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

// runTargetSubCmd executes `target <subcmd> <args...>` against a fake
// daemon at addr through the normal authenticated session path: it writes
// a non-expired login session at ~/.gibson/auth/credentials (HOME is
// redirected to a temp dir) pointing at the fake server over an http://
// URL, so the dial uses plaintext transport and no token refresh.
func runTargetSubCmd(t *testing.T, addr string, subArgs ...string) (string, error) {
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

	cmd := Command()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(subArgs)
	err := cmd.Execute()
	return buf.String(), err
}

func TestCommand_WiresSubcommands(t *testing.T) {
	c := Command()
	want := map[string]bool{"create": false, "list": false, "get": false, "update": false, "delete": false}
	for _, sub := range c.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q not registered under `gibson target`", name)
		}
	}
}

func TestCreate_RequiresName(t *testing.T) {
	c := Command()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"create"})
	err := c.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("want --name required error, got %v", err)
	}
}

// TestTarget_RequiresLogin verifies commands fail closed (with the "run
// gibson login" hint) when no session exists.
func TestTarget_RequiresLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := Command()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000"})
	err := c.Execute()
	require.ErrorIs(t, err, deviceauth.ErrNotLoggedIn)
}

func TestCreate_PrintsMintedUUID(t *testing.T) {
	svc := &fakeDaemonServer{
		createFn: func(_ context.Context, req *daemonv1.CreateTargetRequest) (*daemonv1.CreateTargetResponse, error) {
			require.Equal(t, "victim", req.GetTarget().GetName())
			require.Equal(t, "llm_chat", req.GetTarget().GetType())
			return &daemonv1.CreateTargetResponse{TargetId: "tgt-uuid-1"}, nil
		},
	}
	addr := startFakeDaemonServer(t, svc)

	out, err := runTargetSubCmd(t, addr, "create", "--name", "victim", "--type", "llm_chat")
	require.NoError(t, err)
	require.Contains(t, out, "tgt-uuid-1")
}

func TestList_RendersTargets(t *testing.T) {
	svc := &fakeDaemonServer{
		listFn: func(_ context.Context, _ *daemonv1.ListTargetsRequest) (*daemonv1.ListTargetsResponse, error) {
			return &daemonv1.ListTargetsResponse{
				Targets: []*targetv1.Target{
					{Id: "u1", Name: "alpha", Type: "llm_chat", Status: "active"},
				},
			}, nil
		},
	}
	addr := startFakeDaemonServer(t, svc)

	out, err := runTargetSubCmd(t, addr, "list")
	require.NoError(t, err)
	require.Contains(t, out, "u1")
	require.Contains(t, out, "alpha")
	require.Contains(t, out, "llm_chat")
}

func TestGet_MarshalsTarget(t *testing.T) {
	svc := &fakeDaemonServer{
		getFn: func(_ context.Context, req *daemonv1.GetTargetRequest) (*daemonv1.GetTargetResponse, error) {
			require.Equal(t, "u1", req.GetTargetId())
			return &daemonv1.GetTargetResponse{Target: &targetv1.Target{Id: "u1", Name: "alpha"}}, nil
		},
	}
	addr := startFakeDaemonServer(t, svc)

	out, err := runTargetSubCmd(t, addr, "get", "u1")
	require.NoError(t, err)
	require.Contains(t, out, "alpha")
}

func TestDelete_ConfirmsUUID(t *testing.T) {
	var deleted string
	svc := &fakeDaemonServer{
		deleteFn: func(_ context.Context, req *daemonv1.DeleteTargetRequest) (*daemonv1.DeleteTargetResponse, error) {
			deleted = req.GetTargetId()
			return &daemonv1.DeleteTargetResponse{}, nil
		},
	}
	addr := startFakeDaemonServer(t, svc)

	out, err := runTargetSubCmd(t, addr, "delete", "u1")
	require.NoError(t, err)
	require.Contains(t, out, "deleted u1")
	require.Equal(t, "u1", deleted)
}
