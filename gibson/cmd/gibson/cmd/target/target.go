// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package target groups the `gibson target` subcommands: create, list, get,
// update, delete. Targets are the systems a mission assesses. Each target is
// identified by a server-minted UUID — the name and other fields are metadata.
// Nothing resolves a target by name; missions reference the UUID.
//
// The commands call the customer-facing DaemonService target RPCs
// (CreateTarget / GetTarget / ListTargets / UpdateTarget / DeleteTarget) over
// the authenticated login session established by `gibson login` (bearer token
// + x-gibson-tenant). The daemon URL comes from that session (override with
// --gibson-url); there is no plaintext/unauthenticated path.
package target

import (
	"context"
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	"google.golang.org/protobuf/encoding/protojson"

	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	targetv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/target/v1"
)

// Command returns the `gibson target` parent command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "target",
		Short: "Create, list, inspect, update, and delete assessment targets",
		Long: `gibson target — manage the systems your missions assess.

A target is identified by a server-minted UUID; the name and other fields are
metadata. Missions reference a target by its UUID — names are never resolved.

Subcommands:
  create  register a new target (prints the minted UUID)
  list    list your targets (UUID + metadata)
  get     show a single target by UUID
  update  replace a target's metadata by UUID
  delete  remove a target by UUID`,
		SilenceUsage: true,
	}
	c.AddCommand(createCmd())
	c.AddCommand(listCmd())
	c.AddCommand(getCmd())
	c.AddCommand(updateCmd())
	c.AddCommand(deleteCmd())
	return c
}

// connFlags are the daemon-dial flags shared by every subcommand.
type connFlags struct {
	gibsonURL string
	tenant    string
	timeout   time.Duration
}

func (f *connFlags) bind(c *cobra.Command) {
	c.Flags().StringVar(&f.gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().StringVar(&f.tenant, "tenant", "", "Override the active tenant id for this call.")
	c.Flags().DurationVar(&f.timeout, "timeout", 30*time.Second, "Request deadline")
}

// dial opens a DaemonService client over the authenticated login session.
// The returned cleanup closes the connection.
func (f *connFlags) dial(ctx context.Context) (daemonv1.DaemonServiceClient, func(), context.Context, context.CancelFunc, error) {
	cctx, cancel := context.WithTimeout(ctx, f.timeout)
	conn, err := deviceauth.Dial(cctx, f.gibsonURL, f.tenant)
	if err != nil {
		cancel()
		return nil, nil, nil, nil, err
	}
	cleanup := func() { _ = conn.Close() }
	return daemonv1.NewDaemonServiceClient(conn), cleanup, cctx, cancel, nil
}

func createCmd() *cobra.Command {
	var (
		cf          connFlags
		name        string
		ttype       string
		url         string
		provider    string
		description string
		tags        []string
		targetTO    int32
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Register a new target and print its minted UUID",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return errors.New("--name is required")
			}
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			tgt := &targetv1.Target{
				Name:        name,
				Type:        ttype,
				Url:         url,
				Provider:    provider,
				Description: description,
				Tags:        tags,
				Timeout:     targetTO,
			}
			resp, err := client.CreateTarget(ctx, &daemonv1.CreateTargetRequest{Target: tgt})
			if err != nil {
				return fmt.Errorf("CreateTarget: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.GetTargetId())
			return nil
		},
	}
	cf.bind(c)
	c.Flags().StringVar(&name, "name", "", "Human-readable target name (metadata)")
	c.Flags().StringVar(&ttype, "type", "", "Target type (e.g. llm_chat, custom)")
	c.Flags().StringVar(&url, "url", "", "Target endpoint URL")
	c.Flags().StringVar(&provider, "provider", "", "Backing provider (e.g. openai)")
	c.Flags().StringVar(&description, "description", "", "Free-text description")
	c.Flags().StringSliceVar(&tags, "tag", nil, "Tag (repeatable)")
	c.Flags().Int32Var(&targetTO, "target-timeout", 0, "Per-operation timeout in seconds")
	return c
}

func listCmd() *cobra.Command {
	var (
		cf       connFlags
		provider string
		ttype    string
		status   string
		tags     []string
		limit    int32
		offset   int32
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List your targets (UUID + metadata)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.ListTargets(ctx, &daemonv1.ListTargetsRequest{
				Filter: &targetv1.TargetFilter{
					Provider: provider,
					Type:     ttype,
					Status:   status,
					Tags:     tags,
					Limit:    limit,
					Offset:   offset,
				},
			})
			if err != nil {
				return fmt.Errorf("ListTargets: %w", err)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "UUID\tNAME\tTYPE\tSTATUS")
			for _, t := range resp.GetTargets() {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.GetId(), t.GetName(), t.GetType(), t.GetStatus())
			}
			return w.Flush()
		},
	}
	cf.bind(c)
	c.Flags().StringVar(&provider, "provider", "", "Filter by provider")
	c.Flags().StringVar(&ttype, "type", "", "Filter by type")
	c.Flags().StringVar(&status, "status", "", "Filter by status")
	c.Flags().StringSliceVar(&tags, "tag", nil, "Filter by tag (repeatable; target must carry all)")
	c.Flags().Int32Var(&limit, "limit", 0, "Max results (0 = server default)")
	c.Flags().Int32Var(&offset, "offset", 0, "Skip the first N results")
	return c
}

func getCmd() *cobra.Command {
	var cf connFlags
	c := &cobra.Command{
		Use:   "get <uuid>",
		Short: "Show a single target by UUID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.GetTarget(ctx, &daemonv1.GetTargetRequest{TargetId: args[0]})
			if err != nil {
				return fmt.Errorf("GetTarget: %w", err)
			}
			out, err := (protojson.MarshalOptions{Multiline: true, Indent: "  "}).Marshal(resp.GetTarget())
			if err != nil {
				return fmt.Errorf("protojson marshal: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cf.bind(c)
	return c
}

func updateCmd() *cobra.Command {
	var (
		cf          connFlags
		name        string
		ttype       string
		url         string
		provider    string
		description string
		tags        []string
		targetTO    int32
	)
	c := &cobra.Command{
		Use:   "update <uuid>",
		Short: "Replace a target's metadata by UUID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			resp, err := client.UpdateTarget(ctx, &daemonv1.UpdateTargetRequest{
				Target: &targetv1.Target{
					Id:          args[0],
					Name:        name,
					Type:        ttype,
					Url:         url,
					Provider:    provider,
					Description: description,
					Tags:        tags,
					Timeout:     targetTO,
				},
			})
			if err != nil {
				return fmt.Errorf("UpdateTarget: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.GetTarget().GetId())
			return nil
		},
	}
	cf.bind(c)
	c.Flags().StringVar(&name, "name", "", "Human-readable target name (metadata)")
	c.Flags().StringVar(&ttype, "type", "", "Target type")
	c.Flags().StringVar(&url, "url", "", "Target endpoint URL")
	c.Flags().StringVar(&provider, "provider", "", "Backing provider")
	c.Flags().StringVar(&description, "description", "", "Free-text description")
	c.Flags().StringSliceVar(&tags, "tag", nil, "Tag (repeatable)")
	c.Flags().Int32Var(&targetTO, "target-timeout", 0, "Per-operation timeout in seconds")
	return c
}

func deleteCmd() *cobra.Command {
	var cf connFlags
	c := &cobra.Command{
		Use:   "delete <uuid>",
		Short: "Remove a target by UUID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, ctx, cancel, err := cf.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()
			defer cancel()

			if _, err := client.DeleteTarget(ctx, &daemonv1.DeleteTargetRequest{TargetId: args[0]}); err != nil {
				return fmt.Errorf("DeleteTarget: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
	cf.bind(c)
	return c
}
