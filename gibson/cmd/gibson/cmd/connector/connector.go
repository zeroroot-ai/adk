// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package connector implements `gibson connector` subcommands: catalog,
// enable, list, disable. All commands call ConnectorService on the Gibson
// daemon (through Envoy) to manage the tenant connector lifecycle
// (ADR-0014 Slice 4). A person enables a connector from the curated catalog
// and it becomes a running connector the connector-operator reconciles onto
// ToolHive; the person does not author YAML. Authentication is the human
// login session set up by `gibson login` (bearer token + x-gibson-tenant);
// there is no unauthenticated path.
package connector

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	tenantv1 "github.com/zeroroot-ai/adk/gibson/internal/genproto/gibson/tenant/v1"
	"google.golang.org/grpc"
)

// Command returns the `gibson connector` parent command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "connector",
		Short: "Manage tenant connectors (ConnectorService)",
		Long: `gibson connector — enable and manage connectors via
ConnectorService on the Gibson daemon (ADR-0014 Slice 4). Pick a connector
from the curated catalog and it becomes a running connector the operator
reconciles onto ToolHive. You do not write YAML. Authenticated as you: run ` +
			"`gibson login`" + ` first.

Subcommands:
  catalog  list the curated connectors this tenant may enable
  enable   enable a connector from the catalog by its catalog id
  list     list the tenant's enabled connectors and their live status
  disable  disable an enabled connector (removes the running connector)`,
		SilenceUsage: true,
	}
	c.AddCommand(catalogCmd())
	c.AddCommand(enableCmd())
	c.AddCommand(listCmd())
	c.AddCommand(disableCmd())
	return c
}

// session loads the human login session and opens an authenticated
// connection to the daemon. gibsonURL (when non-empty) overrides the
// session's stored URL; tenant (when non-empty) overrides the active
// tenant for this call. It delegates to the shared deviceauth.Dial entry
// point every tenant-scoped command group uses.
func session(ctx context.Context, gibsonURL, tenant string) (*grpc.ClientConn, error) {
	conn, err := deviceauth.Dial(ctx, gibsonURL, tenant)
	if err != nil {
		return nil, fmt.Errorf("connect to the daemon: %w", err)
	}
	return conn, nil
}

func catalogCmd() *cobra.Command {
	var (
		gibsonURL string
		tenant    string
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "catalog",
		Short: "List the curated connectors this tenant may enable",
		Long: `Call ConnectorService.ListCatalog and print the curated
connectors this tenant may enable. The set is the shipped catalog, filtered
by the per-tenant catalog gate. Enable one with ` + "`gibson connector enable <id>`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			resp, err := tenantv1.NewConnectorServiceClient(conn).ListCatalog(ctx, &tenantv1.ListCatalogRequest{})
			if err != nil {
				return fmt.Errorf("ListCatalog: %w", err)
			}
			if len(resp.GetEntries()) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(no catalog entries)")
				return nil
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "%-20s  %-24s  %-8s  %-6s  %s\n", "ID", "NAME", "SHAPE", "AUTH", "DESCRIPTION")
			for _, e := range resp.GetEntries() {
				_, _ = fmt.Fprintf(w, "%-20s  %-24s  %-8s  %-6s  %s\n",
					e.GetId(), e.GetDisplayName(), e.GetShape(), e.GetAuth(), e.GetDescription())
			}
			return nil
		},
	}
	bindDaemonFlags(c, &gibsonURL, &tenant, &timeout)
	return c
}

func enableCmd() *cobra.Command {
	var (
		gibsonURL string
		tenant    string
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "enable <catalog-id>",
		Short: "Enable a connector from the catalog",
		Long: `Call ConnectorService.EnableConnector for a catalog entry
(for example ` + "`gibson connector enable gitlab`" + `). The daemon creates
the connector in your tenant and the operator reconciles it onto ToolHive.
An OAuth connector comes up in the AuthorizationRequired phase until a person
authorizes it. The enabled connector id and its initial phase are printed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			resp, err := tenantv1.NewConnectorServiceClient(conn).EnableConnector(ctx, &tenantv1.EnableConnectorRequest{
				CatalogId: args[0],
			})
			if err != nil {
				return fmt.Errorf("EnableConnector: %w", err)
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "connector: %s\n", resp.GetConnector())
			_, _ = fmt.Fprintf(w, "phase:     %s\n", resp.GetPhase())
			return nil
		},
	}
	bindDaemonFlags(c, &gibsonURL, &tenant, &timeout)
	return c
}

func listCmd() *cobra.Command {
	var (
		gibsonURL string
		tenant    string
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List the tenant's enabled connectors and their live status",
		Long: `Call ConnectorService.ListConnectors and print the tenant's
enabled connectors with their live status: shape, runtime, lifecycle phase,
the count of discovered tools, and the last error (if any).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			resp, err := tenantv1.NewConnectorServiceClient(conn).ListConnectors(ctx, &tenantv1.ListConnectorsRequest{})
			if err != nil {
				return fmt.Errorf("ListConnectors: %w", err)
			}
			if len(resp.GetConnectors()) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(no connectors)")
				return nil
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "%-20s  %-8s  %-10s  %-22s  %-5s  %s\n",
				"CONNECTOR", "SHAPE", "RUNTIME", "PHASE", "TOOLS", "LAST ERROR")
			for _, cn := range resp.GetConnectors() {
				_, _ = fmt.Fprintf(w, "%-20s  %-8s  %-10s  %-22s  %-5d  %s\n",
					cn.GetId(), cn.GetShape(), cn.GetRuntime(), cn.GetPhase(),
					cn.GetDiscoveredTools(), cn.GetLastError())
			}
			return nil
		},
	}
	bindDaemonFlags(c, &gibsonURL, &tenant, &timeout)
	return c
}

func disableCmd() *cobra.Command {
	var (
		gibsonURL string
		tenant    string
		timeout   time.Duration
		yes       bool
	)
	c := &cobra.Command{
		Use:   "disable <connector>",
		Short: "Disable an enabled connector",
		Long: `Call ConnectorService.DisableConnector to disable an enabled
connector by its id. The operator cascade-removes the running connector, its
network policy, and its credential secret. This action is destructive: it
asks you to confirm unless you pass --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connectorID := args[0]
			if !yes {
				ok, err := confirm(cmd, connectorID)
				if err != nil {
					return err
				}
				if !ok {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := session(ctx, gibsonURL, tenant)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			if _, err := tenantv1.NewConnectorServiceClient(conn).DisableConnector(ctx, &tenantv1.DisableConnectorRequest{
				Connector: connectorID,
			}); err != nil {
				return fmt.Errorf("DisableConnector: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "disabled %s\n", connectorID)
			return nil
		},
	}
	bindDaemonFlags(c, &gibsonURL, &tenant, &timeout)
	c.Flags().BoolVar(&yes, "yes", false, "Do not ask to confirm the disable")
	return c
}

// confirm asks the operator to confirm a destructive disable. It reads one
// line from the command's input and returns true only for "y" or "yes".
func confirm(cmd *cobra.Command, connectorID string) (bool, error) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Disable connector %q? This removes it and its credential secret. [y/N]: ", connectorID)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return false, nil // no input (for example EOF): treat as "no"
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// bindDaemonFlags registers the daemon-connection flags shared by every
// connector subcommand. They mirror the other tenant-scoped command groups so
// GIBSON_URL / login-session / CA handling stay identical across the CLI.
func bindDaemonFlags(c *cobra.Command, gibsonURL, tenant *string, timeout *time.Duration) {
	c.Flags().StringVar(gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().StringVar(tenant, "tenant", "", "Override the active tenant id for this call.")
	c.Flags().DurationVar(timeout, "timeout", 30*time.Second, "RPC deadline")
}
