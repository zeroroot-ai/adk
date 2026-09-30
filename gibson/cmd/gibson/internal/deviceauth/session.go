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
		Endpoint: oauth2.Endpoint{TokenURL: cr.TokenURL, AuthStyle: PublicClientAuthStyle},
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

// rpcAuth attaches the bearer token to every RPC. It never sends a
// tenant header: ADR-0093 (decision 4) and gibson#233 make a person's
// tenant a fact of their verified Zitadel org, resolved by the daemon
// from the bearer token alone, so ext-authz refuses a client-supplied
// x-gibson-tenant for an OIDC user. RequireTransportSecurity is false
// so the same creds work against a local plaintext daemon and TLS
// Envoy alike; the bearer is only sent because the caller dialed a
// trusted endpoint.
type rpcAuth struct {
	src oauth2.TokenSource
}

func (a rpcAuth) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	tok, err := a.src.Token()
	if err != nil {
		return nil, fmt.Errorf("deviceauth: obtain access token (run `gibson login`): %w", err)
	}
	return map[string]string{"authorization": "Bearer " + tok.AccessToken}, nil
}

func (a rpcAuth) RequireTransportSecurity() bool { return false }

// DialDaemon opens a gRPC connection to the daemon through Envoy using
// the stored human session's bearer token. The daemon resolves the
// caller's tenant itself from that token; the CLI never sends one.
func (cr *Credentials) DialDaemon(ctx context.Context) (*grpc.ClientConn, error) {
	addr, useTLS, err := dialAddress(cr.GibsonURL)
	if err != nil {
		return nil, err
	}
	opts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(rpcAuth{src: cr.TokenSource(ctx)}),
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
// when non-empty. It is the shared entry point for tenant-scoped CLI
// commands (provider, target, mission, agent); there is no
// unauthenticated path. The daemon, not the CLI, decides the caller's
// tenant from the bearer token (ADR-0093 decision 4).
func Dial(ctx context.Context, gibsonURL string) (*grpc.ClientConn, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, err // ErrNotLoggedIn carries the "run gibson login" hint
	}
	if gibsonURL != "" {
		creds.GibsonURL = gibsonURL
	}
	return creds.DialDaemon(ctx)
}

// ResolveActiveTenant reads the caller's tenant membership from the
// daemon and returns its canonical id, for display and storage only.
// ADR-0093 (decision 4): a person's tenant is a fact of their verified
// Zitadel org, never a client choice, so the CLI never sends one — the
// daemon derives tenant scope from the bearer token alone, and each
// person belongs to exactly one tenant. When preferred is non-empty it
// is checked against that tenant rather than used to pick one.
func (cr *Credentials) ResolveActiveTenant(ctx context.Context, preferred string) (string, error) {
	conn, err := cr.DialDaemon(ctx)
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
	tenantID, tenantName := ms[0].GetTenantId(), ms[0].GetTenantName()
	if preferred != "" && preferred != tenantID && !strings.EqualFold(tenantName, preferred) {
		return "", fmt.Errorf("deviceauth: your account belongs to tenant %q (%s), not %q", tenantID, tenantName, preferred)
	}
	return tenantID, nil
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
