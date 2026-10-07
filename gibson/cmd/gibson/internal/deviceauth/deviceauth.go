// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package deviceauth implements the human-facing `gibson login` /
// `gibson logout` flow: the OAuth 2.0 Device Authorization Grant
// (RFC 8628) straight against the platform's Zitadel issuer.
//
// The CLI learns its non-secret bootstrap config (issuer, public
// client_id, scopes) from the daemon's `GET {GIBSON_URL}/.well-known/
// gibson-login` endpoint (gibson#623), discovers the OAuth endpoints via
// OIDC discovery, runs the device flow, and persists the resulting
// token set to `~/.gibson/auth/credentials` (mode 0600).
//
// Tenant is NOT carried in the token: it is resolved after login from
// the caller's FGA memberships (DaemonService.ListMyMemberships) and
// stored alongside the token as the active tenant. See ADR-0093.
//
// Design reference: platform-operator#80 (the Zitadel device-grant
// client), adk#115 (this flow). The dashboard-brokered device flow it
// replaces is being deleted (dashboard#718).
package deviceauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// BootstrapPath is the daemon endpoint that publishes the CLI's
// non-secret OAuth bootstrap config, served behind the public
// (no-auth) Envoy route. Joined onto the resolved GIBSON_URL.
const BootstrapPath = "/.well-known/gibson-login"

// DefaultScopes is the fallback scope set when the bootstrap endpoint
// omits one. `offline_access` mints the refresh token; the project-aud
// scope (injected per environment by the bootstrap endpoint) is what
// gets the daemon audience into the token so ext-authz accepts it.
var DefaultScopes = []string{"openid", "profile", "email", "offline_access"}

// Bootstrap is the shape returned by GET {GIBSON_URL}/.well-known/gibson-login.
type Bootstrap struct {
	Issuer   string   `json:"issuer"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes,omitempty"`
}

// discovery is the subset of the OIDC discovery document we need.
type discovery struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
}

// Client performs the OAuth 2.0 device-authorization flow. Its HTTP field
// is the injection point for tests; production uses http.DefaultClient.
type Client struct {
	HTTP *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// FetchBootstrap retrieves the CLI's bootstrap config from the daemon.
// gibsonURL is the resolved GIBSON_URL (Envoy base); the endpoint is
// public and unauthenticated.
func (c *Client) FetchBootstrap(ctx context.Context, gibsonURL string) (*Bootstrap, error) {
	endpoint, err := joinURL(gibsonURL, BootstrapPath)
	if err != nil {
		return nil, fmt.Errorf("deviceauth: build bootstrap URL: %w", err)
	}
	var b Bootstrap
	if err := c.getJSON(ctx, endpoint, &b); err != nil {
		return nil, fmt.Errorf("deviceauth: fetch bootstrap (%s): %w", endpoint, err)
	}
	if b.Issuer == "" || b.ClientID == "" {
		return nil, fmt.Errorf("deviceauth: bootstrap from %s missing issuer or client_id", endpoint)
	}
	if len(b.Scopes) == 0 {
		b.Scopes = DefaultScopes
	}
	return &b, nil
}

// Discover fetches the OIDC discovery document for issuer and returns
// an oauth2.Endpoint wired for the device flow.
func (c *Client) Discover(ctx context.Context, issuer string) (oauth2.Endpoint, error) {
	endpoint, err := joinURL(issuer, "/.well-known/openid-configuration")
	if err != nil {
		return oauth2.Endpoint{}, fmt.Errorf("deviceauth: build discovery URL: %w", err)
	}
	var d discovery
	if err := c.getJSON(ctx, endpoint, &d); err != nil {
		return oauth2.Endpoint{}, fmt.Errorf("deviceauth: OIDC discovery (%s): %w", endpoint, err)
	}
	// OIDC Discovery 1.0 section 4.3: the `issuer` in the document MUST be
	// identical to the issuer URL used to fetch it. Without this check a
	// document served from one host can name another issuer, and the CLI then
	// uses that host's authorization and token endpoints while believing it is
	// talking to the issuer the daemon's bootstrap named. The issuer is also
	// what the token's `iss` is validated against downstream, so a mismatch
	// here is a mismatch nothing later catches.
	//
	// Trailing slashes are normalised because a document may spell its own
	// issuer with or without one and both denote the same issuer.
	if got, want := strings.TrimSuffix(d.Issuer, "/"), strings.TrimSuffix(issuer, "/"); got != want {
		return oauth2.Endpoint{}, fmt.Errorf(
			"deviceauth: OIDC discovery at %s declares issuer %q, expected %q — "+
				"the daemon's bootstrap named one issuer and its discovery document names another, "+
				"so the login would use endpoints from an issuer nobody asked for",
			endpoint, d.Issuer, issuer)
	}
	if d.TokenEndpoint == "" || d.DeviceAuthorizationEndpoint == "" {
		return oauth2.Endpoint{}, fmt.Errorf("deviceauth: issuer %s does not advertise a device_authorization_endpoint (is the device grant enabled?)", issuer)
	}
	return oauth2.Endpoint{
		AuthURL:       d.AuthorizationEndpoint,
		TokenURL:      d.TokenEndpoint,
		DeviceAuthURL: d.DeviceAuthorizationEndpoint,
		AuthStyle:     PublicClientAuthStyle,
	}, nil
}

// PublicClientAuthStyle sends client_id in the form body, as RFC 8628
// section 3.4 describes for a public client. The oauth2 default probes
// instead: after each failed token request it sends the same request
// again with the other auth style. Every authorization_pending answer
// then costs two requests against the edge's per-client login quota.
const PublicClientAuthStyle = oauth2.AuthStyleInParams

// Config assembles the oauth2.Config for the device flow from a
// bootstrap + discovered endpoint.
func Config(b *Bootstrap, endpoint oauth2.Endpoint) *oauth2.Config {
	return &oauth2.Config{
		ClientID: b.ClientID,
		Endpoint: endpoint,
		Scopes:   b.Scopes,
	}
}

// rateLimitFallbackWait is the pause after a 429 that names no reset time.
const rateLimitFallbackWait = 10 * time.Second

// PollToken polls the token endpoint until the user approves the device
// request. It wraps oauth2's DeviceAccessToken, which treats any HTTP
// 429 as fatal because a 429 carries no OAuth error code. The platform
// edge counts the poll against a per-client login quota, so a 429 is a
// normal signal here. PollToken waits until the quota window resets and
// then polls again. The context and the device code expiry bound the
// total wait.
func PollToken(ctx context.Context, cfg *oauth2.Config, da *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	for {
		tok, err := cfg.DeviceAccessToken(ctx, da)
		wait, limited := rateLimitWait(err)
		if !limited {
			if err != nil {
				if codeExpired(ctx, da, err) {
					return nil, ErrDeviceCodeExpired
				}
				return nil, fmt.Errorf("deviceauth: poll token: %w", err)
			}
			return tok, nil
		}
		if !da.Expiry.IsZero() && time.Now().Add(wait).After(da.Expiry) {
			return nil, fmt.Errorf("device code expired while rate limited: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("deviceauth: poll token: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
}

// ErrDeviceCodeExpired is returned by PollToken when nobody approved the
// login before the device code lifetime ended.
var ErrDeviceCodeExpired = errors.New("the login code expired before it was approved: run `gibson login` again")

// codeExpired reports whether err means the device code ran out. oauth2
// bounds the poll with a context deadline at da.Expiry, so the expiry
// shows as context.DeadlineExceeded. That case differs from the caller's
// own deadline: the caller's context is still live when the code expires.
// The token endpoint can also answer expired_token.
func codeExpired(ctx context.Context, da *oauth2.DeviceAuthResponse, err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.ErrorCode == "expired_token" {
		return true
	}
	if ctx.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return !da.Expiry.IsZero() && !time.Now().Before(da.Expiry)
}

// rateLimitWait reports whether err is an HTTP 429 from the token
// endpoint, and how long to wait before the next poll. It reads
// Retry-After first, then the edge's X-RateLimit-Reset. Both carry
// seconds.
func rateLimitWait(err error) (time.Duration, bool) {
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) || re.Response == nil || re.Response.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	for _, h := range []string{"Retry-After", "X-RateLimit-Reset"} {
		if n, perr := strconv.Atoi(strings.TrimSpace(re.Response.Header.Get(h))); perr == nil && n >= 0 {
			return time.Duration(n) * time.Second, true
		}
	}
	return rateLimitFallbackWait, true
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}

func joinURL(base, path string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid base URL %q", base)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}

// ---- credential store: ~/.gibson/auth/credentials (mode 0600) ----

// Credentials is the on-disk shape of `~/.gibson/auth/credentials`. It
// holds everything needed to attach auth to subsequent calls and to
// silently refresh, plus the tenant resolved at login for display. It
// is mode 0600 and lives outside workspace.yaml (which forbids token
// fields).
type Credentials struct {
	Issuer       string    `json:"issuer"`
	ClientID     string    `json:"client_id"`
	TokenURL     string    `json:"token_url"`
	Scopes       []string  `json:"scopes,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
	// ActiveTenant is the tenant id resolved from the caller's Zitadel
	// org at login (ResolveActiveTenant), kept for display only. It is
	// never sent to the daemon: ADR-0093 (decision 4) makes tenant scope
	// a fact of the bearer token, not a client-supplied value.
	ActiveTenant string `json:"active_tenant,omitempty"`
	GibsonURL    string `json:"gibson_url"`

	// CACertPath is a PEM file holding a CA to trust in addition to the system
	// store, for an install that terminates TLS with its own internal CA
	// (adk#178). Persisted with the session so it survives across commands;
	// --ca-cert and GIBSON_CA_CERT override it per invocation.
	CACertPath string `json:"ca_cert_path,omitempty"`
}

// CredentialsPath returns `~/.gibson/auth/credentials`.
func CredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("deviceauth: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gibson", "auth", "credentials"), nil
}

// Save writes creds to ~/.gibson/auth/credentials with mode 0600,
// creating the parent directory (0700) if needed. It writes a new file of
// mode 0600 and renames it over the old one, so a file that was wider before
// ends at mode 0600 too.
func (cr *Credentials) Save() error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("deviceauth: create auth dir: %w", err)
	}
	b, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return fmt.Errorf("deviceauth: marshal credentials: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("deviceauth: create credentials file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("deviceauth: set credentials file mode: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("deviceauth: write credentials: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("deviceauth: write credentials: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("deviceauth: replace credentials: %w", err)
	}
	return nil
}

// LoadCredentials reads ~/.gibson/auth/credentials. Returns
// ErrNotLoggedIn if the file does not exist.
func LoadCredentials() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotLoggedIn
		}
		return nil, fmt.Errorf("deviceauth: read credentials: %w", err)
	}
	var cr Credentials
	if err := json.Unmarshal(b, &cr); err != nil {
		return nil, fmt.Errorf("deviceauth: parse credentials: %w", err)
	}
	return &cr, nil
}

// Delete removes the credential file. Missing file is not an error
// (logout is idempotent).
func Delete() error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deviceauth: remove credentials: %w", err)
	}
	return nil
}

// ErrNotLoggedIn is returned when no credential file exists.
var ErrNotLoggedIn = errors.New("not logged in (run `gibson login`)")

// Token reconstructs an oauth2.Token from the stored credentials.
func (cr *Credentials) Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  cr.AccessToken,
		RefreshToken: cr.RefreshToken,
		Expiry:       cr.Expiry,
	}
}
