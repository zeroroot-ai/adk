// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package auth implements `gibson login` and `gibson logout` — the
// human device-authorization flow against the platform's Zitadel
// issuer. See cmd/gibson/internal/deviceauth for the underlying client.
package auth

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/workspace"
)

// LoginCommand returns the `gibson login` command.
func LoginCommand() *cobra.Command {
	var (
		gibsonURL string
		issuer    string
		clientID  string
		tenant    string
		noBrowser bool
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "login",
		Short: "Authenticate the CLI against the Gibson platform (device flow)",
		Long: `login runs the OAuth 2.0 Device Authorization Grant against the
platform's identity service: it prints a URL + short code, you approve
in a browser, and the CLI stores the resulting session at
~/.gibson/auth/credentials (mode 0600). Every subsequent gibson command
then acts as you. The session refreshes silently; run gibson logout to
end it.

The CLI learns its issuer + public client_id from the platform
(GET {GIBSON_URL}/.well-known/gibson-login); pass --issuer/--client-id to
override for local or air-gapped setups. GIBSON_URL defaults to
https://api.zeroroot.ai; --gibson-url, the GIBSON_URL env var, or a
workspace file (gibson init) override it.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := workspace.Resolve(gibsonURL)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			// Honour a private CA (adk#178) on the LOGIN flow itself: the
			// bootstrap fetch, OIDC discovery, and the device-token polling
			// all hit the install's own TLS edge, which is exactly where a
			// private CA lives. http.DefaultClient here made --ca-cert a flag
			// that worked for every command except the first one anyone runs.
			caCertPath := deviceauth.CACertPath("")
			hc, err := deviceauth.HTTPClient(caCertPath)
			if err != nil {
				return err
			}
			dc := &deviceauth.Client{HTTP: hc}
			// The oauth2 package reads its HTTP client from the context.
			ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)

			// Bootstrap: issuer + client_id, from flags or the daemon.
			b := &deviceauth.Bootstrap{Issuer: issuer, ClientID: clientID}
			if b.Issuer == "" || b.ClientID == "" {
				fetched, err := dc.FetchBootstrap(ctx, res.GibsonURL)
				if err != nil {
					return fmt.Errorf("%w\n(pass --issuer and --client-id to skip the platform bootstrap)", err)
				}
				if b.Issuer == "" {
					b.Issuer = fetched.Issuer
				}
				if b.ClientID == "" {
					b.ClientID = fetched.ClientID
				}
				b.Scopes = fetched.Scopes
			}
			if len(b.Scopes) == 0 {
				b.Scopes = deviceauth.DefaultScopes
			}

			endpoint, err := dc.Discover(ctx, b.Issuer)
			if err != nil {
				return err
			}
			cfg := deviceauth.Config(b, endpoint)

			da, err := cfg.DeviceAuth(ctx)
			if err != nil {
				return fmt.Errorf("login: start device authorization: %w", err)
			}

			w := cmd.OutOrStdout()
			verify := da.VerificationURIComplete
			if verify == "" {
				verify = da.VerificationURI
			}
			fmt.Fprintf(w, "\nTo finish signing in, open:\n  %s\n", verify)
			fmt.Fprintf(w, "and confirm this code:  %s\n\n", da.UserCode)
			if !noBrowser {
				if err := openBrowser(verify); err != nil {
					_, _ = fmt.Fprintf(w, "Not opening a browser: %v\n", err)
				}
			}
			fmt.Fprintln(w, "Waiting for approval...")

			tok, err := deviceauth.PollToken(ctx, cfg, da)
			if err != nil {
				return fmt.Errorf("login: %w", err)
			}

			creds := &deviceauth.Credentials{
				Issuer:       b.Issuer,
				ClientID:     b.ClientID,
				TokenURL:     endpoint.TokenURL,
				Scopes:       b.Scopes,
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				Expiry:       tok.Expiry,
				ActiveTenant: tenant,
				GibsonURL:    res.GibsonURL,
				// Persisted so every later command — including the silent
				// token refresh against the issuer — trusts the same CA the
				// login did, without re-passing the flag.
				CACertPath: caCertPath,
			}
			if err := creds.Save(); err != nil {
				return err
			}
			fmt.Fprintf(w, "\nLogged in. Session stored at ~/.gibson/auth/credentials.\n")

			// Resolve the tenant from the caller's FGA membership
			// (DaemonService.ListMyMemberships), for display only: the
			// daemon derives the real tenant scope from the bearer token
			// on every call (ADR-0093 decision 4), so this never changes
			// what the CLI is authorized to do. Best-effort: a transient
			// daemon error or a --tenant mismatch is reported but does not
			// undo a successful login — the token is already saved.
			resolved, rerr := creds.ResolveActiveTenant(ctx, tenant)
			if rerr != nil {
				_, _ = fmt.Fprintf(w, "\nSigned in, but could not confirm your tenant:\n  %v\n", rerr)
				return nil
			}
			if resolved != creds.ActiveTenant {
				creds.ActiveTenant = resolved
				if err := creds.Save(); err != nil {
					return err
				}
			}
			fmt.Fprintf(w, "Active tenant: %s\n", resolved)
			return nil
		},
	}
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Gibson platform URL; falls back to GIBSON_URL, then the workspace, then "+
		workspace.DefaultGibsonURL+".")
	c.Flags().StringVar(&issuer, "issuer", "", "Override the OIDC issuer (skips platform bootstrap).")
	c.Flags().StringVar(&clientID, "client-id", "", "Override the CLI OAuth client_id (skips platform bootstrap).")
	c.Flags().StringVar(&tenant, "tenant", "", "Assert the tenant you expect to sign in as; checked against your "+
		"account, not selected by this flag.")
	c.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not attempt to open a browser; just print the URL.")
	c.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Overall deadline for the login flow.")
	return c
}

// LogoutCommand returns the `gibson logout` command.
func LogoutCommand() *cobra.Command {
	c := &cobra.Command{
		Use:          "logout",
		Short:        "End the CLI session and remove stored credentials",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := deviceauth.Delete(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out. Local session removed.")
			return nil
		},
	}
	return c
}

// openBrowser best-effort opens url in the user's default browser. A
// failure is non-fatal: the URL is already printed for manual use.
func openBrowser(raw string) error {
	url, err := browserURL(raw)
	if err != nil {
		return err
	}
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// browserURL returns the verification URL that the identity provider sent,
// when it is safe to hand to the OS URI handler: https, or http to a loopback
// host. Any other scheme (file:, javascript:, a custom handler) is refused.
// The URL is still printed for the user to open by hand.
func browserURL(raw string) (string, error) {
	u, err := neturl.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("the verification URL does not parse: %w", err)
	}
	switch {
	case u.Scheme == "https" && u.Host != "":
		return u.String(), nil
	case u.Scheme == "http" && isLoopbackHost(u.Hostname()):
		return u.String(), nil
	default:
		return "", fmt.Errorf("the verification URL has scheme %q; only https, or http to a loopback host, is opened", u.Scheme)
	}
}

// isLoopbackHost reports whether host is localhost, an address in
// 127.0.0.0/8, or ::1.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
