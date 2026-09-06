// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package agent implements `gibson agent` subcommands: enroll, list,
// revoke. All commands call AgentIdentityService on the Gibson daemon
// (through Envoy) to manage machine identities for agents, tools, and
// plugins. Authentication is the human login session established by
// `gibson login` (bearer token + x-gibson-tenant); there is no
// plaintext/unauthenticated path.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	agentidentityv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/agentidentity/v1"
	"google.golang.org/grpc"
)

// Command returns the `gibson agent` parent command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent/tool/plugin machine identities (AgentIdentityService)",
		Long: `gibson agent — provision and manage machine identities for
agents, tools, and plugins via AgentIdentityService on the Gibson
daemon. Authenticated as you: run ` + "`gibson login`" + ` first.

Subcommands:
  enroll  provision a new machine identity and print the one-time secret
  list    list all identities for the tenant
  revoke  permanently revoke an identity by principal ID`,
		SilenceUsage: true,
	}
	c.AddCommand(agentEnrollCmd())
	c.AddCommand(agentListCmd())
	c.AddCommand(agentRevokeCmd())
	return c
}

// session loads the human login session and opens an authenticated
// connection to the daemon. gibsonURL (when non-empty) overrides the
// session's stored URL; tenant (when non-empty) overrides the active
// tenant for this call. It delegates to the shared deviceauth.Dial entry
// point used by every tenant-scoped command group.
func session(ctx context.Context, gibsonURL, tenant string) (*grpc.ClientConn, error) {
	return deviceauth.Dial(ctx, gibsonURL, tenant)
}

// parsePrincipalKind maps the --kind flag to the proto enum.
func parsePrincipalKind(s string) (agentidentityv1.PrincipalKind, error) {
	switch strings.ToLower(s) {
	case "agent", "":
		return agentidentityv1.PrincipalKind_PRINCIPAL_KIND_AGENT, nil
	case "tool":
		return agentidentityv1.PrincipalKind_PRINCIPAL_KIND_TOOL, nil
	case "plugin":
		return agentidentityv1.PrincipalKind_PRINCIPAL_KIND_PLUGIN, nil
	default:
		return agentidentityv1.PrincipalKind_PRINCIPAL_KIND_UNSPECIFIED,
			fmt.Errorf("unknown kind %q: want agent | tool | plugin", s)
	}
}

func agentEnrollCmd() *cobra.Command {
	var (
		gibsonURL   string
		tenant      string
		timeout     time.Duration
		name        string
		kind        string
		description string
		capability  []string
	)
	c := &cobra.Command{
		Use:   "enroll",
		Short: "Provision a new machine identity and print the one-time bootstrap token",
		Long: `Call AgentIdentityService.CreateAgentIdentity to provision a new
machine identity, authenticated as you (run ` + "`gibson login`" + ` first).
The one-time bootstrap token is printed to stdout — store it immediately;
it cannot be retrieved again. Run the printed enroll_command (or
` + "`gibson component register --token`" + `) to complete the capability-grant
handshake (ADR-0045). The same flow serves every component kind.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			principalKind, err := parsePrincipalKind(kind)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			resp, err := agentidentityv1.NewAgentIdentityServiceClient(conn).CreateAgentIdentity(ctx, &agentidentityv1.CreateAgentIdentityRequest{
				Name:              name,
				Kind:              principalKind,
				Description:       description,
				CapabilityCeiling: capability,
			})
			if err != nil {
				return fmt.Errorf("CreateAgentIdentity: %w", err)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "principal_id:  %s\n", resp.GetPrincipalId())
			// Capability-grant bootstrap token (ADR-0045): the one-time credential
			// `gibson component register --token` presents to the CG register
			// endpoint for first registration. It is the sole enrollment credential
			// for every component kind (gibson#670).
			if bt := resp.GetBootstrapToken(); bt != "" {
				fmt.Fprintf(w, "bootstrap_token: %s\n", bt)
			}
			if u := resp.GetGibsonUrl(); u != "" {
				fmt.Fprintf(w, "gibson_url:    %s\n", u)
			}
			if ec := resp.GetEnrollCommand(); ec != "" {
				fmt.Fprintf(w, "\nenroll_command:\n  %s\n", ec)
			}
			return nil
		},
	}
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().StringVar(&tenant, "tenant", "", "Override the active tenant id for this call.")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "RPC deadline")
	c.Flags().StringVar(&name, "name", "", "Identity name (required)")
	c.Flags().StringVar(&kind, "kind", "agent", "Component kind: agent | tool | plugin")
	c.Flags().StringVar(&description, "description", "", "Human-readable description")
	c.Flags().StringArrayVar(&capability, "capability", nil,
		"Session capability to grant in the credential ceiling; repeatable. "+
			"Reserved names: mission:originate (start its own mission), mission:delegate")
	_ = c.MarkFlagRequired("name")
	return c
}

func agentListCmd() *cobra.Command {
	var (
		gibsonURL string
		tenant    string
		timeout   time.Duration
		kind      string
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List all machine identities for the tenant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			req := &agentidentityv1.ListAgentIdentitiesRequest{}
			if kind != "" {
				pk, err := parsePrincipalKind(kind)
				if err != nil {
					return err
				}
				req.KindFilter = pk
			}

			resp, err := agentidentityv1.NewAgentIdentityServiceClient(conn).ListAgentIdentities(ctx, req)
			if err != nil {
				return fmt.Errorf("ListAgentIdentities: %w", err)
			}
			if len(resp.GetIdentities()) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no identities)")
				return nil
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-36s  %-8s  %-24s  %s\n", "PRINCIPAL ID", "KIND", "NAME", "REVOKED")
			for _, id := range resp.GetIdentities() {
				kindStr := strings.ToLower(strings.TrimPrefix(id.GetKind().String(), "PRINCIPAL_KIND_"))
				revoked := ""
				if id.GetRevoked() {
					revoked = "revoked"
				}
				fmt.Fprintf(w, "%-36s  %-8s  %-24s  %s\n", id.GetPrincipalId(), kindStr, id.GetName(), revoked)
			}
			return nil
		},
	}
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().StringVar(&tenant, "tenant", "", "Override the active tenant id for this call.")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "RPC deadline")
	c.Flags().StringVar(&kind, "kind", "", "Filter by kind: agent | tool | plugin (default: all)")
	return c
}

func agentRevokeCmd() *cobra.Command {
	var (
		gibsonURL string
		tenant    string
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "revoke <principal-id>",
		Short: "Permanently revoke a machine identity",
		Long: `Call AgentIdentityService.RevokeAgentIdentity. The identity is
immediately invalidated; existing JWTs for the principal stop being
accepted by the daemon. This action is irreversible.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			if _, err := agentidentityv1.NewAgentIdentityServiceClient(conn).RevokeAgentIdentity(ctx, &agentidentityv1.RevokeAgentIdentityRequest{
				PrincipalId: args[0],
			}); err != nil {
				return fmt.Errorf("RevokeAgentIdentity: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "revoked")
			return nil
		},
	}
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().StringVar(&tenant, "tenant", "", "Override the active tenant id for this call.")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "RPC deadline")
	return c
}
