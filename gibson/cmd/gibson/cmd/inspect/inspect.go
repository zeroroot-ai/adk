// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package inspect implements `gibson inspect`.
//
// inspect loads the local runtime credential (auto-detecting agent / tool /
// plugin from the on-disk layout, or GIBSON_AGENT_KEY), signs a per-RPC
// Capability-Grant JWT with the registered agent key, calls the daemon's
// IdentityService.WhoAmI over the x-capability-grant header, and renders the
// caller's effective grants as a human-friendly tree or proto-JSON (ADR-0045).
//
// Spec: component-bootstrap-e2e Requirement 11.
package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/enroll"
	identitypb "github.com/zeroroot-ai/sdk/api/gen/gibson/identity/v1"
	"github.com/zeroroot-ai/sdk/capabilitygrant"
)

// Command returns the `inspect` Cobra command.
func Command() *cobra.Command {
	var (
		kind    string
		name    string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show what this principal can do (calls WhoAmI on the Gibson daemon)",
		Long: `inspect loads the local runtime credential at
~/.gibson/<kind>/<name>.runtime.json (written by gibson component
register), signs a per-RPC Capability-Grant JWT with the registered
agent key, and calls IdentityService.WhoAmI to print the principal's
effective Gibson permissions.

Auto-detection: when --kind is unset, inspect scans ~/.gibson/{agent,
tool,plugin}/*.runtime.json and, when exactly one exists, picks that.
Multiple installs require --kind (and --name). GIBSON_AGENT_KEY (a
base64 runtime credential) overrides the on-disk lookup for CI / k8s.

Output formats:
  default  human-friendly tree with stable action labels for grep
  --json   raw WhoAmIResponse as canonical proto-JSON (for scripts)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInspect(context.Background(), kind, name, jsonOut, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "agent | tool | plugin (auto-detected when only one exists)")
	cmd.Flags().StringVar(&name, "name", "", "Install name when multiple of the same kind exist")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit raw WhoAmIResponse JSON instead of the tree")
	return cmd
}

func runInspect(ctx context.Context, kind, name string, jsonOut bool, out, errOut interface{}) error {
	stdout := mustWriter(out)
	stderr := mustWriter(errOut)

	rc, gibsonURL, detectedKind, err := resolveInstall(kind, name)
	if err != nil {
		return err
	}

	resp, err := callWhoAmI(ctx, rc, gibsonURL)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}

	if jsonOut {
		// Canonical proto-JSON for scripting.
		marshaler := protojson.MarshalOptions{Indent: "  ", UseProtoNames: true}
		b, jerr := marshaler.Marshal(resp)
		if jerr != nil {
			return fmt.Errorf("inspect: marshal json: %w", jerr)
		}
		_, _ = stdout.Write(b)
		_, _ = stdout.Write([]byte("\n"))
		return nil
	}

	renderTree(stdout, resp, detectedKind)
	preflightWarn(stderr, resp, detectedKind)
	return nil
}

// resolveInstall determines which registered component to inspect and loads its
// runtime credential (ADR-0045). The GIBSON_AGENT_KEY env override wins; else
// --kind/--name select the install, falling back to auto-detection across the
// three kinds. A missing credential is a hard error (no auto re-register).
func resolveInstall(kind, name string) (capabilitygrant.RuntimeCredential, string, string, error) {
	// Env override: the credential comes from GIBSON_AGENT_KEY regardless of
	// on-disk installs. Kind defaults to the flag (or "agent") for rendering.
	if os.Getenv(enroll.EnvRuntimeCredential) != "" {
		k := kind
		if k == "" {
			k = "agent"
		}
		rc, url, err := enroll.ResolveRuntimeCredential(k, name)
		return rc, url, k, err
	}

	k, n := kind, name
	if k == "" {
		installs, err := enroll.ListInstalls()
		if err != nil {
			return capabilitygrant.RuntimeCredential{}, "", "", err
		}
		switch len(installs) {
		case 0:
			return capabilitygrant.RuntimeCredential{}, "", "", errors.New("inspect: no registered components found under ~/.gibson/{agent,tool,plugin}/ — run `gibson component register --token <bootstrap-token>` first")
		case 1:
			k, n = installs[0].Kind, installs[0].Name
		default:
			var sb strings.Builder
			sb.WriteString("inspect: multiple registered components found; pass --kind (and --name) to disambiguate:\n")
			for _, in := range installs {
				fmt.Fprintf(&sb, "  --kind %s --name %s\n", in.Kind, in.Name)
			}
			return capabilitygrant.RuntimeCredential{}, "", "", errors.New(sb.String())
		}
	}

	rc, url, err := enroll.ResolveRuntimeCredential(k, n)
	return rc, url, k, err
}

func callWhoAmI(ctx context.Context, rc capabilitygrant.RuntimeCredential, gibsonURL string) (*identitypb.WhoAmIResponse, error) {
	perRPC, err := rc.PerRPCCredentials()
	if err != nil {
		return nil, fmt.Errorf("build CG credentials: %w", err)
	}

	dialAddr, useTLS, err := dialAddressFromURL(gibsonURL)
	if err != nil {
		return nil, err
	}

	dialOpts := []grpc.DialOption{grpc.WithPerRPCCredentials(perRPC)}
	if useTLS {
		// Same private-CA support as the session dial path; a dial site that
		// skipped it would make --ca-cert work for some commands and not others
		// (adk#178).
		tlsCfg, tlsErr := deviceauth.TLSConfig(deviceauth.CACertPath(""))
		if tlsErr != nil {
			return nil, tlsErr
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(dialAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("could not dial Gibson at %s: %w", gibsonURL, err)
	}
	defer conn.Close()

	client := identitypb.NewIdentityServiceClient(conn)
	resp, err := client.WhoAmI(ctx, &identitypb.WhoAmIRequest{})
	if err != nil {
		return nil, fmt.Errorf("WhoAmI failed: %w", err)
	}
	return resp, nil
}

// dialAddressFromURL extracts the host:port for grpc.Dial and reports
// whether TLS is required. Defaults: https→443, http→80.
func dialAddressFromURL(s string) (string, bool, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", false, fmt.Errorf("parse gibson_url: %w", err)
	}
	host := u.Host
	if host == "" {
		return "", false, fmt.Errorf("gibson_url has no host: %s", s)
	}
	if !strings.Contains(host, ":") {
		switch u.Scheme {
		case "http":
			host += ":80"
		default:
			host += ":443"
		}
	}
	return host, u.Scheme == "https", nil
}

// renderTree prints the human-friendly view. Action labels (read,
// write, execute) are fixed strings to keep grep-based parsing stable.
func renderTree(out interface{ Write([]byte) (int, error) }, resp *identitypb.WhoAmIResponse, kind string) {
	fmt.Fprintf(stdoutWriter(out), "%s (kind=%s, principal_id=%s)\n",
		resp.GetName(), strings.ToLower(strings.TrimPrefix(resp.GetKind().String(), "PRINCIPAL_KIND_")), resp.GetPrincipalId())
	fmt.Fprintf(stdoutWriter(out), "  tenant: %s\n", resp.GetTenantId())

	fmt.Fprintln(stdoutWriter(out), "  components:")
	if len(resp.GetComponentGrants()) == 0 {
		fmt.Fprintln(stdoutWriter(out), "    (none)")
	} else {
		grants := append([]*identitypb.ComponentGrantEffective(nil), resp.GetComponentGrants()...)
		sort.Slice(grants, func(i, j int) bool { return grants[i].GetComponentRef() < grants[j].GetComponentRef() })
		maxName := 0
		for _, g := range grants {
			if l := len(g.GetComponentRef()); l > maxName {
				maxName = l
			}
		}
		for _, g := range grants {
			actions := []string{}
			if g.GetCanRead() {
				actions = append(actions, "read")
			}
			if g.GetCanConfigure() {
				actions = append(actions, "write")
			}
			if g.GetCanExecute() {
				actions = append(actions, "execute")
			}
			source := "direct"
			if len(g.GetSources()) > 0 {
				source = sourceLabel(g.GetSources()[0])
			}
			fmt.Fprintf(stdoutWriter(out), "    %-*s  %s  (%s)\n",
				maxName, g.GetComponentRef(), strings.Join(actions, " "), source)
		}
	}

	fmt.Fprintln(stdoutWriter(out), "  plugins:")
	switch {
	case kind == "agent":
		fmt.Fprintln(stdoutWriter(out), "    (none — agents do not invoke plugins directly)")
	case len(resp.GetPluginGrants()) == 0:
		fmt.Fprintln(stdoutWriter(out), "    (none)")
	default:
		for _, p := range resp.GetPluginGrants() {
			fmt.Fprintf(stdoutWriter(out), "    %s\n", p.GetPluginRef())
		}
	}

	fmt.Fprintf(stdoutWriter(out), "  active capability grants: %d\n", len(resp.GetActiveCapabilityGrants()))
}

func sourceLabel(s *identitypb.GrantSource) string {
	switch s.GetKind() {
	case identitypb.GrantSource_KIND_DIRECT:
		return "direct"
	case identitypb.GrantSource_KIND_TENANT_MEMBER:
		return fmt.Sprintf("via %s#member", s.GetSourceObject())
	case identitypb.GrantSource_KIND_TEAM_MEMBER:
		return fmt.Sprintf("via %s#member", s.GetSourceObject())
	case identitypb.GrantSource_KIND_OWNER:
		return fmt.Sprintf("via %s#owner", s.GetSourceObject())
	case identitypb.GrantSource_KIND_UNSPECIFIED:
		return "unknown"
	default:
		return "unknown"
	}
}

// preflightWarn surfaces the "this agent has effectively no grants"
// warning per Requirement 11.5.
func preflightWarn(stderr interface{ Write([]byte) (int, error) }, resp *identitypb.WhoAmIResponse, kind string) {
	if len(resp.GetComponentGrants()) > 0 {
		return
	}
	if len(resp.GetPluginGrants()) > 0 {
		return
	}
	fmt.Fprintf(stdoutWriter(stderr),
		"WARN: this %s has no direct grants and only inherits tenant-member access; check the registration step\n",
		kind)
}

// mustWriter is a small assertion that the cobra-passed writer is
// usable. We avoid importing io.Writer at the public boundary to keep
// the test surface small.
func mustWriter(v interface{}) interface{ Write([]byte) (int, error) } {
	w, ok := v.(interface{ Write([]byte) (int, error) })
	if !ok {
		// Fall back to stderr-shaped writer; main always passes a
		// real io.Writer, this branch is purely defensive.
		return os.Stderr
	}
	return w
}

func stdoutWriter(v interface{ Write([]byte) (int, error) }) interface{ Write([]byte) (int, error) } {
	return v
}
