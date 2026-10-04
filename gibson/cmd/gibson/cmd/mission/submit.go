// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	missionv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/mission/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

func submitCmd() *cobra.Command {
	var (
		formatHint     string
		targetOverride string
		dryRun         bool
		gibsonURL      string
		timeout        time.Duration
		detach         bool
		catalogName    string
		catalogParams  []string
	)
	c := &cobra.Command{
		Use:   "submit [file]",
		Short: "Validate, define, and run a mission via the daemon",
		Long: `Full CUE/YAML/JSON → define → run submit:

1. Load the mission. Either from a FILE (CUE / YAML / JSON detected
   from the extension or --format), or from the platform's own
   catalog with --catalog <name> --param k=v, which renders it
   server-side.
2. Run protovalidate on the parsed *missionv1.MissionDefinition.
3. With --dry-run, print the rendered JSON and exit; otherwise:
4. Call DaemonService/CreateMissionDefinition to register the
   definition and obtain a mission_definition_id.
5. Call DaemonService/CreateMission with the definition ID.
6. Print the returned mission ID.

A catalog mission is rendered by the daemon from the definition
compiled into its own binary (ADR-0018), so the graph is the
checked-in one rather than a copy. Its parameters are a closed set:
run ` + "`gibson mission catalog list`" + ` to see what each one
declares. A catalog mission declares no target, so --target is
required for it.

The daemon URL is taken from your login session (run ` + "`gibson login`" + `
first); override it with --gibson-url. Production use should
route through the dashboard's Server Action path — this command
is the CLI escape hatch for development and CI.

Auth: the call is made over the authenticated login session
(bearer token only; the daemon resolves your tenant from your
account, not from this command); there is no plaintext or
unauthenticated path.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Exactly one source. Both would be two answers to "what runs",
			// and neither leaves nothing to run; saying so here beats a
			// confusing failure three calls later.
			switch {
			case len(args) == 1 && catalogName != "":
				return errors.New("pass a file or --catalog, not both: they are two ways of saying what to run")
			case len(args) == 0 && catalogName == "":
				return errors.New("pass a mission file, or --catalog <name> (list them with: gibson mission catalog list)")
			case len(args) == 1 && len(catalogParams) > 0:
				// Dropping them would run the file while the caller believed
				// their parameters had bound.
				return errors.New("--param applies to --catalog: a mission file has no parameters to bind")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			// The connection is needed up front for a catalog mission, because
			// the definition lives in the daemon. A file still needs none until
			// after --dry-run.
			var conn *grpc.ClientConn
			var client daemonv1.DaemonServiceClient
			dial := func() error {
				if client != nil {
					return nil
				}
				c, err := deviceauth.Dial(ctx, gibsonURL)
				if err != nil {
					return err
				}
				conn = c
				client = daemonv1.NewDaemonServiceClient(conn)
				return nil
			}
			defer func() {
				if conn != nil {
					_ = conn.Close()
				}
			}()

			var def *missionv1.MissionDefinition
			var err error
			if catalogName != "" {
				if err = dial(); err != nil {
					return err
				}
				params, perr := parseParams(catalogParams)
				if perr != nil {
					return perr
				}
				// Rendered SERVER-SIDE. The parameter set is closed there, so a
				// typo is refused rather than dropped, and the daemon is the
				// one that holds the authoritative graph.
				rendered, rerr := client.RenderCatalogMission(ctx, &daemonv1.RenderCatalogMissionRequest{
					Name:   catalogName,
					Params: params,
				})
				if rerr != nil {
					return fmt.Errorf("RenderCatalogMission: %w", rerr)
				}
				def = rendered.GetMission()
			} else {
				def, err = loadMissionFile(args[0], formatHint)
				if err != nil {
					return err
				}
			}
			v, err := protovalidate.New()
			if err != nil {
				return fmt.Errorf("protovalidate.New: %w", err)
			}
			if err := v.Validate(def); err != nil {
				return fmt.Errorf("validate: %w", err)
			}

			if dryRun {
				marshalOpts := protojson.MarshalOptions{
					Multiline: true,
					Indent:    "  ",
				}
				out, err := marshalOpts.Marshal(def)
				if err != nil {
					return fmt.Errorf("protojson marshal: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			if err = dial(); err != nil {
				return err
			}

			// Resolve the target BEFORE anything is created server-side. The
			// daemon requires one:
			// CreateMission rejects an empty target_id outright. It was never
			// sent at all, so `mission submit` failed for EVERY mission with
			// "target_id is required" — the definition's own target_ref was
			// parsed, validated, rendered, and then dropped on the floor at the
			// one call that needed it.
			//
			// Checked here rather than after CreateMissionDefinition, because
			// checking later registered the definition and THEN failed: a
			// targetless submit left an orphan definition behind, and the next
			// attempt with the same name collided with AlreadyExists. A local
			// requirement must not cost a server-side write.
			//
			// After the dial, not before, so a logged-out user hears "run gibson
			// login" — the prerequisite for the whole command — rather than being
			// sent to fix a target they cannot use yet.
			//
			// --target overrides the definition, which is what makes a
			// definition reusable across targets without editing the file.
			targetID := targetOverride
			if targetID == "" {
				targetID = def.GetTargetRef()
			}
			if targetID == "" {
				return fmt.Errorf(
					"no target: set `target_ref` in the mission definition or pass --target <id> " +
						"(create one with `gibson target create --url ...`)")
			}
			if err := checkTargetRef(targetID); err != nil {
				return fmt.Errorf("validate: %w", err)
			}

			// Step 1: register the parsed definition to obtain a
			// stable mission_definition_id.
			defResp, err := client.CreateMissionDefinition(ctx, &daemonv1.CreateMissionDefinitionRequest{
				Definition: def,
			})
			if err != nil {
				return fmt.Errorf("CreateMissionDefinition: %w", err)
			}

			// Step 2: create and start the mission run using the
			// definition ID obtained above.
			resp, err := client.CreateMission(ctx, &daemonv1.CreateMissionRequest{
				Name:                def.GetName(),
				Description:         def.GetDescription(),
				MissionDefinitionId: defResp.GetMissionDefinitionId(),
				TargetId:            targetID,
			})
			if err != nil {
				return fmt.Errorf("CreateMission: %w", err)
			}
			// CreateMission only registers a PENDING record — it does not start
			// anything. This command advertises "validate + define + run", and
			// without this step a submitted mission sits Pending forever with no
			// error to explain why. RunMission is what actually dispatches: the
			// daemon find-or-creates the run, stamps the tenant, and streams
			// lifecycle events.
			runStream, err := client.RunMission(ctx, &daemonv1.RunMissionRequest{
				MissionDefinitionId: defResp.GetMissionDefinitionId(),
				TargetId:            targetID,
			})
			if err != nil {
				return fmt.Errorf("RunMission: %w", err)
			}
			// --detach: wait for the first frame (the daemon accepted and started
			// the run), print the id, and return. The mission keeps running: the
			// daemon detaches the run from this RPC's context (mission_manager
			// newMissionContext uses context.WithoutCancel). A caller that is
			// itself a component of the mission needs this: the plugin submits
			// the session mission as the person and must go on to claim the
			// dispatch, which it cannot do while blocked on this stream.
			if detach {
				if _, recvErr := runStream.Recv(); recvErr != nil && !errors.Is(recvErr, io.EOF) {
					return fmt.Errorf("RunMission: %w", recvErr)
				}
				fmt.Fprintln(cmd.OutOrStdout(), resp.GetMission().GetId())
				return nil
			}
			// Drain the event stream rather than returning after the first
			// frame: holding the stream until the server closes it is what
			// "run" means here; --timeout bounds the wait. A node's error text
			// travels only in these frames, so print it.
			for {
				ev, recvErr := runStream.Recv()
				if errors.Is(recvErr, io.EOF) {
					break
				}
				if recvErr != nil {
					return fmt.Errorf("RunMission: %w", recvErr)
				}
				if t := ev.GetEventType(); t != "" {
					if e := ev.GetError(); e != "" {
						fmt.Fprintf(cmd.ErrOrStderr(), "  %s %s error=%s\n", t, ev.GetMessage(), e)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "  %s %s\n", t, ev.GetMessage())
					}
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), resp.GetMission().GetId())
			return nil
		},
	}
	c.Flags().StringVar(&formatHint, "format", "", "Override input format detection: cue|yaml|json")
	c.Flags().StringVar(&targetOverride, "target", "", "Target id to bind the mission to (overrides the definition's target_ref)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Print rendered JSON; do not contact the daemon")
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Override the daemon URL (defaults to the login session).")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Submit deadline")
	c.Flags().BoolVar(&detach, "detach", false,
		"Return once the daemon has started the run and print the mission id; do not wait for the mission to finish")
	c.Flags().StringVar(&catalogName, "catalog", "",
		"Run a mission from the platform's catalog by name instead of from a file (list them with: gibson mission catalog list)")
	c.Flags().StringArrayVar(&catalogParams, "param", nil,
		"A --catalog mission's parameter, as key=value. Repeatable. The set is closed: an unknown key is refused")
	return c
}

// parseParams turns repeated key=value flags into the map the daemon validates
// against what the mission declares.
//
// A duplicate key is an error rather than last-wins. Two values for one
// parameter is a caller that does not know which it sent, and silently keeping
// one of them is how a run ends up pointed somewhere nobody chose.
func parseParams(in []string) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for _, raw := range in {
		key, value, found := strings.Cut(raw, "=")
		if !found {
			return nil, fmt.Errorf("--param %q is not key=value", raw)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("--param %q has no key", raw)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("--param %s was given twice", key)
		}
		out[key] = value
	}
	return out, nil
}
