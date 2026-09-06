// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package auth implements `gibson login` and `gibson logout` — the
// human device-authorization flow against the platform's Zitadel
// issuer. See cmd/gibson/internal/deviceauth for the underlying client.
package auth

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
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
override for local or air-gapped setups.`,
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
				_ = openBrowser(verify)
			}
			fmt.Fprintln(w, "Waiting for approval...")

			tok, err := cfg.DeviceAccessToken(ctx, da)
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

			// Resolve the active tenant from the caller's FGA memberships
			// (DaemonService.ListMyMemberships). Best-effort: a multi-tenant
			// ambiguity or transient daemon error is reported but does not
			// undo a successful login — the token is already saved.
			resolved, rerr := creds.ResolveActiveTenant(ctx, tenant)
			if rerr != nil {
				fmt.Fprintf(w, "\nSigned in, but could not pin an active tenant:\n  %v\nRe-run `gibson login --tenant <id>` once you know which.\n", rerr)
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
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Gibson platform URL; falls back to env / workspace.")
	c.Flags().StringVar(&issuer, "issuer", "", "Override the OIDC issuer (skips platform bootstrap).")
	c.Flags().StringVar(&clientID, "client-id", "", "Override the CLI OAuth client_id (skips platform bootstrap).")
	c.Flags().StringVar(&tenant, "tenant", "", "Active tenant slug to pin for this session.")
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
func openBrowser(url string) error {
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
