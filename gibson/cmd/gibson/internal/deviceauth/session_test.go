// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package deviceauth

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestRpcAuthNeverSendsTenantHeader guards ADR-0093 (decision 4) and
// gibson#233: a person's tenant is a fact of their verified Zitadel org,
// resolved by the daemon from the bearer token alone. ext-authz refuses a
// client-supplied x-gibson-tenant for an OIDC user (hosted run
// 36395197998), so the CLI must never attach one. This test fails on a
// build that still carries a tenant field on rpcAuth.
func TestRpcAuthNeverSendsTenantHeader(t *testing.T) {
	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-access-token"})
	a := rpcAuth{src: src}

	md, err := a.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata: %v", err)
	}

	if _, ok := md["x-gibson-tenant"]; ok {
		t.Fatalf("GetRequestMetadata sent x-gibson-tenant: %+v", md)
	}
	if got, want := md["authorization"], "Bearer test-access-token"; got != want {
		t.Fatalf("authorization = %q, want %q", got, want)
	}
	if len(md) != 1 {
		t.Fatalf("GetRequestMetadata returned %d keys, want 1 (authorization only): %+v", len(md), md)
	}
}

// TestDialDaemonNeverSendsTenantHeader is the end-to-end guard: even a
// session with a tenant already resolved and persisted from a previous
// `gibson login` (ActiveTenant) must not put x-gibson-tenant on the wire.
// Before this fix, DialDaemon defaulted an empty per-call tenant to
// cr.ActiveTenant and rpcAuth attached it to every RPC — the exact path
// that made ext-authz refuse `gibson agent enroll` with PermissionDenied
// (hosted run 36395197998, zerocool-plugins#10). It captures the incoming
// metadata of a real RPC over a real (plaintext) gRPC connection opened by
// DialDaemon, so it exercises the whole client stack, not just rpcAuth.
func TestDialDaemonNeverSendsTenantHeader(t *testing.T) {
	var got metadata.MD
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		got, _ = metadata.FromIncomingContext(stream.Context())
		return status.Error(codes.Unimplemented, "test stub: no real service registered")
	}))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	creds := &Credentials{
		AccessToken: "at",
		Expiry:      time.Now().Add(time.Hour),
		// A resolved, persisted tenant from a prior login: the exact state
		// that used to leak onto the wire as x-gibson-tenant.
		ActiveTenant: "acme-tenant",
		GibsonURL:    "http://" + lis.Addr().String(),
	}

	conn, err := creds.DialDaemon(context.Background())
	if err != nil {
		t.Fatalf("DialDaemon: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The method and message shape do not matter: UnknownServiceHandler
	// captures the incoming metadata for any call before it ever fails.
	_ = conn.Invoke(ctx, "/test.Probe/AnyMethod", &emptypb.Empty{}, &emptypb.Empty{})

	if _, ok := got["x-gibson-tenant"]; ok {
		t.Fatalf("RPC carried x-gibson-tenant despite a persisted ActiveTenant: %+v", got)
	}
	if auth := got.Get("authorization"); len(auth) == 0 {
		t.Fatal("RPC carried no authorization header")
	}
}

// TestRpcAuthRequireTransportSecurityFalse locks in that the bearer is
// attached over both plaintext (local daemon) and TLS (Envoy) dials,
// since the auth decision belongs to the endpoint the caller dialed.
func TestRpcAuthRequireTransportSecurityFalse(t *testing.T) {
	a := rpcAuth{src: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"})}
	if a.RequireTransportSecurity() {
		t.Fatal("RequireTransportSecurity() = true, want false")
	}
}
