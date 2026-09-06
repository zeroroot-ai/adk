// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package connector

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	tenantv1 "github.com/zeroroot-ai/adk/gibson/internal/genproto/gibson/tenant/v1"
	"google.golang.org/grpc"
)

// fakeConnectorServer stubs the four ConnectorService RPCs.
type fakeConnectorServer struct {
	tenantv1.UnimplementedConnectorServiceServer

	catalogFn func(context.Context, *tenantv1.ListCatalogRequest) (*tenantv1.ListCatalogResponse, error)
	enableFn  func(context.Context, *tenantv1.EnableConnectorRequest) (*tenantv1.EnableConnectorResponse, error)
	listFn    func(context.Context, *tenantv1.ListConnectorsRequest) (*tenantv1.ListConnectorsResponse, error)
	disableFn func(context.Context, *tenantv1.DisableConnectorRequest) (*tenantv1.DisableConnectorResponse, error)
}

func (s *fakeConnectorServer) ListCatalog(ctx context.Context, req *tenantv1.ListCatalogRequest) (*tenantv1.ListCatalogResponse, error) {
	return s.catalogFn(ctx, req)
}

func (s *fakeConnectorServer) EnableConnector(ctx context.Context, req *tenantv1.EnableConnectorRequest) (*tenantv1.EnableConnectorResponse, error) {
	return s.enableFn(ctx, req)
}

func (s *fakeConnectorServer) ListConnectors(ctx context.Context, req *tenantv1.ListConnectorsRequest) (*tenantv1.ListConnectorsResponse, error) {
	return s.listFn(ctx, req)
}

func (s *fakeConnectorServer) DisableConnector(ctx context.Context, req *tenantv1.DisableConnectorRequest) (*tenantv1.DisableConnectorResponse, error) {
	return s.disableFn(ctx, req)
}

func startFakeConnectorServer(t *testing.T, svc tenantv1.ConnectorServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	tenantv1.RegisterConnectorServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

// runConnectorSubCmd executes `connector <subcmd> <args...>` against a fake
// daemon at addr. It writes a non-expired login session (HOME redirected to a
// temp dir) pointing at the fake server, so the commands dial it through the
// normal authenticated session path. stdin supplies the disable confirmation.
func runConnectorSubCmd(t *testing.T, addr, stdin string, subArgs ...string) (string, error) {
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
	cmd.SetIn(strings.NewReader(stdin))
	require.NotEmpty(t, subArgs, "runConnectorSubCmd: no subcommand provided")
	cmd.SetArgs(subArgs)
	err := cmd.Execute()
	return buf.String(), err
}

// TestConnectorRequiresLogin verifies the commands fail closed (with the
// "run gibson login" hint) when no session exists.
func TestConnectorRequiresLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := Command()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"list"})
	require.ErrorIs(t, cmd.Execute(), deviceauth.ErrNotLoggedIn)
}

func TestConnectorCatalog(t *testing.T) {
	addr := startFakeConnectorServer(t, &fakeConnectorServer{
		catalogFn: func(_ context.Context, _ *tenantv1.ListCatalogRequest) (*tenantv1.ListCatalogResponse, error) {
			return &tenantv1.ListCatalogResponse{Entries: []*tenantv1.CatalogEntry{
				{Id: "gitlab", DisplayName: "GitLab", Shape: "Remote", Auth: "oauth", Description: "GitLab MCP server"},
			}}, nil
		},
	})
	out, err := runConnectorSubCmd(t, addr, "", "catalog")
	require.NoError(t, err)
	require.Contains(t, out, "ID")
	require.Contains(t, out, "gitlab")
	require.Contains(t, out, "GitLab")
	require.Contains(t, out, "Remote")
	require.Contains(t, out, "oauth")
}

func TestConnectorEnable(t *testing.T) {
	addr := startFakeConnectorServer(t, &fakeConnectorServer{
		enableFn: func(_ context.Context, req *tenantv1.EnableConnectorRequest) (*tenantv1.EnableConnectorResponse, error) {
			require.Equal(t, "gitlab", req.GetCatalogId())
			return &tenantv1.EnableConnectorResponse{Connector: "gitlab", Phase: "AuthorizationRequired"}, nil
		},
	})
	out, err := runConnectorSubCmd(t, addr, "", "enable", "gitlab")
	require.NoError(t, err)
	require.Contains(t, out, "connector: gitlab")
	require.Contains(t, out, "phase:     AuthorizationRequired")
}

func TestConnectorList(t *testing.T) {
	addr := startFakeConnectorServer(t, &fakeConnectorServer{
		listFn: func(_ context.Context, _ *tenantv1.ListConnectorsRequest) (*tenantv1.ListConnectorsResponse, error) {
			return &tenantv1.ListConnectorsResponse{Connectors: []*tenantv1.Connector{
				{Id: "gitlab", Shape: "Remote", Runtime: "mcp", Phase: "Ready", DiscoveredTools: 7},
			}}, nil
		},
	})
	out, err := runConnectorSubCmd(t, addr, "", "list")
	require.NoError(t, err)
	require.Contains(t, out, "CONNECTOR")
	require.Contains(t, out, "gitlab")
	require.Contains(t, out, "Ready")
	require.Contains(t, out, "7")
}

func TestConnectorDisableConfirmYes(t *testing.T) {
	var called bool
	addr := startFakeConnectorServer(t, &fakeConnectorServer{
		disableFn: func(_ context.Context, req *tenantv1.DisableConnectorRequest) (*tenantv1.DisableConnectorResponse, error) {
			called = true
			require.Equal(t, "gitlab", req.GetConnector())
			return &tenantv1.DisableConnectorResponse{}, nil
		},
	})
	// --yes skips the prompt.
	out, err := runConnectorSubCmd(t, addr, "", "disable", "gitlab", "--yes")
	require.NoError(t, err)
	require.True(t, called)
	require.Contains(t, out, "disabled gitlab")
}

func TestConnectorDisablePromptConfirms(t *testing.T) {
	var called bool
	addr := startFakeConnectorServer(t, &fakeConnectorServer{
		disableFn: func(_ context.Context, _ *tenantv1.DisableConnectorRequest) (*tenantv1.DisableConnectorResponse, error) {
			called = true
			return &tenantv1.DisableConnectorResponse{}, nil
		},
	})
	out, err := runConnectorSubCmd(t, addr, "yes\n", "disable", "gitlab")
	require.NoError(t, err)
	require.True(t, called)
	require.Contains(t, out, "disabled gitlab")
}

func TestConnectorDisablePromptAborts(t *testing.T) {
	var called bool
	addr := startFakeConnectorServer(t, &fakeConnectorServer{
		disableFn: func(_ context.Context, _ *tenantv1.DisableConnectorRequest) (*tenantv1.DisableConnectorResponse, error) {
			called = true
			return &tenantv1.DisableConnectorResponse{}, nil
		},
	})
	out, err := runConnectorSubCmd(t, addr, "n\n", "disable", "gitlab")
	require.NoError(t, err)
	require.False(t, called, "DisableConnector must not be called when the operator declines")
	require.Contains(t, out, "aborted")
}
