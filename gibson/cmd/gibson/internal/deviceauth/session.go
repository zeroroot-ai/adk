// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package deviceauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	daemonpb "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TokenSource returns an oauth2.TokenSource that refreshes the stored
// human session token using the persisted refresh token, and writes
// the rotated token set back to ~/.gibson/auth/credentials so the next
// command reuses it. The CLI client is public (no secret).
func (cr *Credentials) TokenSource(ctx context.Context) oauth2.TokenSource {
	cfg := &oauth2.Config{
		ClientID: cr.ClientID,
		Endpoint: oauth2.Endpoint{TokenURL: cr.TokenURL},
		Scopes:   cr.Scopes,
	}
	// The silent refresh hits the issuer's token endpoint, which sits behind
	// the same private CA as everything else on the install (adk#178). An
	// unreadable CA file falls through to the system pool: the refresh then
	// fails with a TLS error naming the issuer, which is the best available
	// signal from a signature that cannot return one.
	if hc, err := HTTPClient(CACertPath(cr.CACertPath)); err == nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	}
	base := cfg.TokenSource(ctx, cr.Token())
	return &persistingSource{base: base, creds: cr}
}

// persistingSource saves rotated tokens back to disk on refresh.
type persistingSource struct {
	base  oauth2.TokenSource
	creds *Credentials
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != p.creds.AccessToken {
		p.creds.AccessToken = tok.AccessToken
		if tok.RefreshToken != "" {
			p.creds.RefreshToken = tok.RefreshToken
		}
		p.creds.Expiry = tok.Expiry
		_ = p.creds.Save() // best-effort; a failed save just means a re-refresh next time
	}
	return tok, nil
}

// rpcAuth attaches the bearer token and x-gibson-tenant header to every
// RPC. RequireTransportSecurity is false so the same creds work against
// a local plaintext daemon and TLS Envoy alike; the bearer is only sent
// because the caller dialed a trusted endpoint.
type rpcAuth struct {
	src    oauth2.TokenSource
	tenant string
}

func (a rpcAuth) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	tok, err := a.src.Token()
	if err != nil {
		return nil, fmt.Errorf("deviceauth: obtain access token (run `gibson login`): %w", err)
	}
	md := map[string]string{"authorization": "Bearer " + tok.AccessToken}
	if a.tenant != "" {
		md["x-gibson-tenant"] = a.tenant
	}
	return md, nil
}

func (a rpcAuth) RequireTransportSecurity() bool { return false }

// DialDaemon opens a gRPC connection to the daemon through Envoy using
// the stored human session: bearer token + the given active tenant
// (falls back to the session's stored ActiveTenant when tenant == "").
func (cr *Credentials) DialDaemon(ctx context.Context, tenant string) (*grpc.ClientConn, error) {
	if tenant == "" {
		tenant = cr.ActiveTenant
	}
	addr, useTLS, err := dialAddress(cr.GibsonURL)
	if err != nil {
		return nil, err
	}
	opts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(rpcAuth{src: cr.TokenSource(ctx), tenant: tenant}),
	}
	if useTLS {
		// Honour a private CA (adk#178): an install terminating TLS with its own
		// internal CA is the normal enterprise case, and the system pool alone
		// cannot verify it.
		tlsCfg, tlsErr := TLSConfig(CACertPath(cr.CACertPath))
		if tlsErr != nil {
			return nil, tlsErr
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("deviceauth: dial daemon at %s: %w", addr, err)
	}
	return conn, nil
}

// Dial loads the stored login session and opens an authenticated gRPC
// connection to the daemon. gibsonURL overrides the session's stored URL
// when non-empty; tenant overrides the active tenant for this call when
// non-empty. It is the shared entry point for tenant-scoped CLI commands
// (provider, target, mission, agent); there is no unauthenticated path.
func Dial(ctx context.Context, gibsonURL, tenant string) (*grpc.ClientConn, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, err // ErrNotLoggedIn carries the "run gibson login" hint
	}
	if gibsonURL != "" {
		creds.GibsonURL = gibsonURL
	}
	return creds.DialDaemon(ctx, tenant)
}

// ResolveActiveTenant picks the tenant for this session. If preferred
// is non-empty it is validated against the caller's memberships;
// otherwise: 0 memberships -> error, 1 -> that one, >1 -> error asking
// for --tenant. The result is the canonical tenant id.
func (cr *Credentials) ResolveActiveTenant(ctx context.Context, preferred string) (string, error) {
	conn, err := cr.DialDaemon(ctx, preferred)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	resp, err := daemonpb.NewDaemonServiceClient(conn).ListMyMemberships(ctx, &daemonpb.ListMyMembershipsRequest{})
	if err != nil {
		return "", fmt.Errorf("deviceauth: list memberships: %w", err)
	}
	ms := resp.GetMemberships()
	if len(ms) == 0 {
		return "", errors.New("deviceauth: you are not a member of any tenant yet (ask to be invited, or sign up)")
	}
	if preferred != "" {
		for _, m := range ms {
			if m.GetTenantId() == preferred || strings.EqualFold(m.GetTenantName(), preferred) {
				return m.GetTenantId(), nil
			}
		}
		return "", fmt.Errorf("deviceauth: you are not a member of tenant %q", preferred)
	}
	if len(ms) == 1 {
		return ms[0].GetTenantId(), nil
	}
	var b strings.Builder
	b.WriteString("deviceauth: you belong to multiple tenants; pass --tenant <id>:\n")
	for _, m := range ms {
		fmt.Fprintf(&b, "  %s  (%s)\n", m.GetTenantId(), m.GetTenantName())
	}
	return "", fmt.Errorf("%s", b.String())
}

// dialAddress turns a GIBSON_URL into a grpc dial target + TLS flag.
func dialAddress(gibsonURL string) (addr string, useTLS bool, err error) {
	u, err := url.Parse(gibsonURL)
	if err != nil {
		return "", false, fmt.Errorf("deviceauth: parse gibson_url %q: %w", gibsonURL, err)
	}
	host := u.Host
	if host == "" {
		return "", false, fmt.Errorf("deviceauth: gibson_url %q has no host", gibsonURL)
	}
	switch u.Scheme {
	case "https":
		useTLS = true
		if u.Port() == "" {
			host += ":443"
		}
	case "http":
		useTLS = false
		if u.Port() == "" {
			host += ":80"
		}
	default:
		return "", false, fmt.Errorf("deviceauth: gibson_url scheme must be http or https, got %q", u.Scheme)
	}
	return host, useTLS, nil
}
