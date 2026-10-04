// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
)

// catalogCmd lists the missions the platform ships, and shows one.
//
// gibson compiles its first-party mission definitions into its own binary
// (ADR-0018). Until the daemon served them, a person could neither see what
// the platform shipped nor run one of them — the only reader was the
// agent-facing harness callback (gibson#631). So the answer to "what can I
// run" lived in the gibson repository and nowhere a user could reach.
func catalogCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "catalog",
		Short: "List the missions the platform ships, or show one",
		Long: `The platform's own mission definitions, compiled into the daemon.

With no arguments, list them with the parameters each one declares.
With a name, print that mission's CUE — the source the daemon renders,
not a summary of it.

Submit one with:

    gibson mission submit --catalog <name> --target <target-id> --param k=v

The parameter set is closed: an unknown key is refused rather than
dropped, so a typo cannot read as a value that bound.`,
		Args: cobra.MaximumNArgs(1),
	}
	c.AddCommand(catalogListCmd())
	c.AddCommand(catalogShowCmd())
	return c
}

func catalogListCmd() *cobra.Command {
	var (
		gibsonURL string
		timeout   time.Duration
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List the missions the platform ships",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			conn, err := deviceauth.Dial(ctx, gibsonURL)
			if err != nil {
				// Wrapped, and errors.Is still reaches
				// deviceauth.ErrNotLoggedIn — which is what tells a logged-out
				// user to run `gibson login`.
				return fmt.Errorf("dial the daemon: %w", err)
			}
			defer func() { _ = conn.Close() }()

			resp, err := daemonv1.NewDaemonServiceClient(conn).
				ListCatalogMissions(ctx, &daemonv1.ListCatalogMissionsRequest{})
			if err != nil {
				return fmt.Errorf("ListCatalogMissions: %w", err)
			}
			if len(resp.GetMissions()) == 0 {
				// Said out loud. An empty listing printed as nothing reads as a
				// broken command rather than as an empty catalog.
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "the platform ships no missions")
				return nil
			}
			for _, m := range resp.GetMissions() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", m.GetName(), m.GetDescription())
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  version %s\n", m.GetVersion())
				if p := m.GetDeclaredParams(); len(p) > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  --param %s\n", strings.Join(p, "=… --param ")+"=…")
				} else {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  takes no parameters")
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Request deadline")
	return c
}

func catalogShowCmd() *cobra.Command {
	var (
		gibsonURL string
		timeout   time.Duration
		params    []string
		rendered  bool
	)
	c := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a shipped mission's CUE, or its rendered definition",
		Long: `Print the mission's CUE source, verbatim, so what the daemon will
run can be read rather than trusted.

With --rendered, print the rendered MissionDefinition as JSON instead.
That needs every parameter the mission declares, because rendering is
what validates them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			parsed, err := parseParams(params)
			if err != nil {
				return err
			}

			conn, err := deviceauth.Dial(ctx, gibsonURL)
			if err != nil {
				// Wrapped, and errors.Is still reaches
				// deviceauth.ErrNotLoggedIn — which is what tells a logged-out
				// user to run `gibson login`.
				return fmt.Errorf("dial the daemon: %w", err)
			}
			defer func() { _ = conn.Close() }()

			resp, err := daemonv1.NewDaemonServiceClient(conn).
				RenderCatalogMission(ctx, &daemonv1.RenderCatalogMissionRequest{
					Name:   args[0],
					Params: parsed,
				})
			if err != nil {
				return fmt.Errorf("RenderCatalogMission: %w", err)
			}

			if !rendered {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), resp.GetSource())
				return nil
			}
			out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(resp.GetMission())
			if err != nil {
				return fmt.Errorf("protojson marshal: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Request deadline")
	c.Flags().StringArrayVar(&params, "param", nil,
		"A parameter, as key=value. Repeatable. Required with --rendered, because rendering is what validates them")
	c.Flags().BoolVar(&rendered, "rendered", false,
		"Print the rendered MissionDefinition as JSON instead of the mission's CUE")
	return c
}
