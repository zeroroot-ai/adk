// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package inspect

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	identitypb "github.com/zeroroot-ai/sdk/api/gen/gibson/identity/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type whoAmIServer struct {
	identitypb.UnimplementedIdentityServiceServer
	auth string
}

func (s *whoAmIServer) WhoAmI(ctx context.Context, _ *identitypb.WhoAmIRequest) (*identitypb.WhoAmIResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if v := md.Get("authorization"); len(v) > 0 {
		s.auth = v[0]
	}
	if s.auth != "Bearer human-token" {
		return nil, status.Error(codes.Unauthenticated, "bad token")
	}
	return &identitypb.WhoAmIResponse{PrincipalId: "user-1", Name: "user-1", TenantId: "tenant-1"}, nil
}

// TestInspectUsesLoginSession proves inspect calls WhoAmI with the login
// session when no component credential exists (adk#60).
func TestInspectUsesLoginSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIBSON_AGENT_KEY", "")

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	identitypb.RegisterIdentityServiceServer(srv, &whoAmIServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	creds := &deviceauth.Credentials{
		AccessToken: "human-token",
		Expiry:      time.Now().Add(time.Hour),
		GibsonURL:   "http://" + lis.Addr().String(),
	}
	if err := creds.Save(); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	cmd := Command()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(nil)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.Contains(out.String(), "user-1 (kind=") || !strings.Contains(out.String(), "tenant: tenant-1") {
		t.Fatalf("output = %q, want the WhoAmI result", out.String())
	}
}

// TestInspectNoCredentialNoSession proves the error names both ways to sign in.
func TestInspectNoCredentialNoSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIBSON_AGENT_KEY", "")
	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gibson login") {
		t.Fatalf("err = %v, want a hint to run gibson login", err)
	}
}
