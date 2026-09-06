// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agent

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	agentidentityv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/agentidentity/v1"
	"google.golang.org/grpc"
)

// fakeAgentServer stubs the identity-management methods on AgentIdentityService.
type fakeAgentServer struct {
	agentidentityv1.UnimplementedAgentIdentityServiceServer

	enrollFn func(context.Context, *agentidentityv1.CreateAgentIdentityRequest) (*agentidentityv1.CreateAgentIdentityResponse, error)
	listFn   func(context.Context, *agentidentityv1.ListAgentIdentitiesRequest) (*agentidentityv1.ListAgentIdentitiesResponse, error)
	revokeFn func(context.Context, *agentidentityv1.RevokeAgentIdentityRequest) (*agentidentityv1.RevokeAgentIdentityResponse, error)
}

func (s *fakeAgentServer) CreateAgentIdentity(ctx context.Context, req *agentidentityv1.CreateAgentIdentityRequest) (*agentidentityv1.CreateAgentIdentityResponse, error) {
	return s.enrollFn(ctx, req)
}

func (s *fakeAgentServer) ListAgentIdentities(ctx context.Context, req *agentidentityv1.ListAgentIdentitiesRequest) (*agentidentityv1.ListAgentIdentitiesResponse, error) {
	return s.listFn(ctx, req)
}

func (s *fakeAgentServer) RevokeAgentIdentity(ctx context.Context, req *agentidentityv1.RevokeAgentIdentityRequest) (*agentidentityv1.RevokeAgentIdentityResponse, error) {
	return s.revokeFn(ctx, req)
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return lis
}

func startFakeAgentServer(t *testing.T, svc agentidentityv1.AgentIdentityServiceServer) string {
	t.Helper()
	lis := mustListen(t)
	srv := grpc.NewServer()
	agentidentityv1.RegisterAgentIdentityServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

// runAgentSubCmd executes `agent <subcmd> <args...>` against a fake
// daemon at addr. It writes a login session at ~/.gibson/auth/credentials
// (HOME is redirected to a temp dir) pointing at the fake server, so the
// commands dial it through the normal authenticated session path. The
// stored token is non-expired, so the oauth2 token source returns it
// without a network refresh.
func runAgentSubCmd(t *testing.T, addr string, subArgs ...string) (string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	creds := &deviceauth.Credentials{
		Issuer:       "http://127.0.0.1:1", // unused: token is valid, no refresh
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
	if len(subArgs) == 0 {
		t.Fatal("runAgentSubCmd: no subcommand provided")
	}
	cmd.SetArgs(subArgs)
	err := cmd.Execute()
	return buf.String(), err
}

// TestAgentRequiresLogin verifies the commands fail closed (with the
// "run gibson login" hint) when no session exists.
func TestAgentRequiresLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := Command()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()
	require.ErrorIs(t, err, deviceauth.ErrNotLoggedIn)
}

func TestParsePrincipalKind(t *testing.T) {
	cases := []struct {
		input string
		want  agentidentityv1.PrincipalKind
	}{
		{"agent", agentidentityv1.PrincipalKind_PRINCIPAL_KIND_AGENT},
		{"AGENT", agentidentityv1.PrincipalKind_PRINCIPAL_KIND_AGENT},
		{"tool", agentidentityv1.PrincipalKind_PRINCIPAL_KIND_TOOL},
		{"plugin", agentidentityv1.PrincipalKind_PRINCIPAL_KIND_PLUGIN},
		{"", agentidentityv1.PrincipalKind_PRINCIPAL_KIND_AGENT}, // default
	}
	for _, tc := range cases {
		got, err := parsePrincipalKind(tc.input)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestParsePrincipalKind_invalid(t *testing.T) {
	_, err := parsePrincipalKind("robot")
	require.Error(t, err)
	require.Contains(t, err.Error(), "robot")
}

func TestAgentEnroll(t *testing.T) {
	svc := &fakeAgentServer{
		enrollFn: func(_ context.Context, req *agentidentityv1.CreateAgentIdentityRequest) (*agentidentityv1.CreateAgentIdentityResponse, error) {
			require.Equal(t, "my-agent", req.GetName())
			require.Equal(t, agentidentityv1.PrincipalKind_PRINCIPAL_KIND_AGENT, req.GetKind())
			return &agentidentityv1.CreateAgentIdentityResponse{
				PrincipalId:    "pid-001",
				BootstrapToken: "bootstrap-xyz",
				GibsonUrl:      "https://gibson.example.com",
				EnrollCommand:  "gibson component register --kind agent --token -",
			}, nil
		},
	}
	addr := startFakeAgentServer(t, svc)

	out, err := runAgentSubCmd(t, addr, "enroll", "--name", "my-agent")
	require.NoError(t, err)
	require.Contains(t, out, "pid-001")
	require.Contains(t, out, "bootstrap_token: bootstrap-xyz")
	require.Contains(t, out, "gibson.example.com")
	require.Contains(t, out, "enroll_command")
	// The OAuth client_secret path is gone (gibson#670): no secret is printed.
	require.NotContains(t, out, "client_secret")
}

func TestAgentEnroll_toolKind(t *testing.T) {
	svc := &fakeAgentServer{
		enrollFn: func(_ context.Context, req *agentidentityv1.CreateAgentIdentityRequest) (*agentidentityv1.CreateAgentIdentityResponse, error) {
			require.Equal(t, agentidentityv1.PrincipalKind_PRINCIPAL_KIND_TOOL, req.GetKind())
			return &agentidentityv1.CreateAgentIdentityResponse{
				PrincipalId:    "pid-tool",
				BootstrapToken: "tool-bootstrap",
			}, nil
		},
	}
	addr := startFakeAgentServer(t, svc)

	out, err := runAgentSubCmd(t, addr, "enroll", "--name", "my-tool", "--kind", "tool")
	require.NoError(t, err)
	require.Contains(t, out, "pid-tool")
}

func TestAgentList_empty(t *testing.T) {
	svc := &fakeAgentServer{
		listFn: func(_ context.Context, _ *agentidentityv1.ListAgentIdentitiesRequest) (*agentidentityv1.ListAgentIdentitiesResponse, error) {
			return &agentidentityv1.ListAgentIdentitiesResponse{}, nil
		},
	}
	addr := startFakeAgentServer(t, svc)

	out, err := runAgentSubCmd(t, addr, "list")
	require.NoError(t, err)
	require.Contains(t, out, "(no identities)")
}

func TestAgentList_withIdentities(t *testing.T) {
	svc := &fakeAgentServer{
		listFn: func(_ context.Context, _ *agentidentityv1.ListAgentIdentitiesRequest) (*agentidentityv1.ListAgentIdentitiesResponse, error) {
			return &agentidentityv1.ListAgentIdentitiesResponse{
				Identities: []*agentidentityv1.AgentIdentity{
					{PrincipalId: "pid-1", Kind: agentidentityv1.PrincipalKind_PRINCIPAL_KIND_AGENT, Name: "alpha"},
					{PrincipalId: "pid-2", Kind: agentidentityv1.PrincipalKind_PRINCIPAL_KIND_TOOL, Name: "beta", Revoked: true},
				},
			}, nil
		},
	}
	addr := startFakeAgentServer(t, svc)

	out, err := runAgentSubCmd(t, addr, "list")
	require.NoError(t, err)
	require.Contains(t, out, "alpha")
	require.Contains(t, out, "beta")
	require.Contains(t, out, "revoked")
}

func TestAgentList_kindFilter(t *testing.T) {
	var capturedFilter agentidentityv1.PrincipalKind
	svc := &fakeAgentServer{
		listFn: func(_ context.Context, req *agentidentityv1.ListAgentIdentitiesRequest) (*agentidentityv1.ListAgentIdentitiesResponse, error) {
			capturedFilter = req.GetKindFilter()
			return &agentidentityv1.ListAgentIdentitiesResponse{}, nil
		},
	}
	addr := startFakeAgentServer(t, svc)

	_, err := runAgentSubCmd(t, addr, "list", "--kind", "plugin")
	require.NoError(t, err)
	require.Equal(t, agentidentityv1.PrincipalKind_PRINCIPAL_KIND_PLUGIN, capturedFilter)
}

func TestAgentRevoke(t *testing.T) {
	var revoked string
	svc := &fakeAgentServer{
		revokeFn: func(_ context.Context, req *agentidentityv1.RevokeAgentIdentityRequest) (*agentidentityv1.RevokeAgentIdentityResponse, error) {
			revoked = req.GetPrincipalId()
			return &agentidentityv1.RevokeAgentIdentityResponse{}, nil
		},
	}
	addr := startFakeAgentServer(t, svc)

	out, err := runAgentSubCmd(t, addr, "revoke", "pid-to-revoke")
	require.NoError(t, err)
	require.Contains(t, out, "revoked")
	require.Equal(t, "pid-to-revoke", revoked)
}
