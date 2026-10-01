// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/zeroroot-ai/adk/gibson/cmd/gibson/internal/deviceauth"
	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	targetv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/target/v1"
)

// targetRefRx matches the one `target_ref:` line in a template body. Keyed by
// content, not by line number, so adding a node to a template never re-pins
// it. Every shipped template carries exactly one, asserted in new_test.go.
var targetRefRx = regexp.MustCompile(`(?m)^(\s*target_ref:\s*)"[^"]*"`)

// withTarget rewrites a scaffold's target_ref to a resolved target UUID.
func withTarget(body, targetID string) string {
	return targetRefRx.ReplaceAllString(body, `${1}"`+targetID+`"`)
}

// resolveTarget turns what the user asked for into a target UUID.
//
// A UUID is taken as given and needs no daemon. Anything else is a name, and
// an empty request means "the obvious one", so both read the tenant's targets
// and the command refuses rather than guessing when there is no single
// answer. `mission new` scaffolded `target_ref: ""` until now, and `submit`
// refused one command later with "no target" — every shipped template failed
// that way, which is the first thing a new member saw (adk#68). A scaffold
// that cannot run is not a scaffold.
func resolveTarget(ctx context.Context, gibsonURL, want string) (id, name string, err error) {
	if _, perr := uuid.Parse(want); perr == nil {
		return want, "", nil
	}

	conn, err := deviceauth.Dial(ctx, gibsonURL)
	if err != nil {
		return "", "", fmt.Errorf("%w (or pass --target <uuid>, or --no-target to scaffold offline)", err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := daemonv1.NewDaemonServiceClient(conn).ListTargets(ctx, &daemonv1.ListTargetsRequest{})
	if err != nil {
		return "", "", fmt.Errorf("ListTargets: %w", err)
	}
	targets := resp.GetTargets()

	if len(targets) == 0 {
		return "", "", fmt.Errorf(
			"no targets in this tenant: create one with `gibson target create --url ...`, " +
				"or pass --no-target to scaffold a file you fill in yourself")
	}

	if want != "" {
		for _, t := range targets {
			if t.GetName() == want {
				return t.GetId(), t.GetName(), nil
			}
		}
		return "", "", fmt.Errorf("no target named %q; this tenant has:\n%s", want, targetLines(targets))
	}

	if len(targets) == 1 {
		return targets[0].GetId(), targets[0].GetName(), nil
	}
	return "", "", fmt.Errorf(
		"this tenant has %d targets, so pick one with --target <name-or-uuid>:\n%s",
		len(targets), targetLines(targets))
}

func targetLines(targets []*targetv1.Target) string {
	var b strings.Builder
	for _, t := range targets {
		fmt.Fprintf(&b, "  %s  %s\n", t.GetId(), t.GetName())
	}
	return strings.TrimRight(b.String(), "\n")
}

// builtinTemplates ships a v1 set of inline CUE templates so
// `mission new --from-template <name>` works without an OCI
// bundle pull. The bundle-fetched templates from
// mission-authoring-cue Requirement 7 will supersede these once
// the OCI puller lands; until then, these are the canonical
// templates the CLI offers.
var builtinTemplates = map[string]builtinTemplate{
	"recon": {
		Synopsis: "Network + service reconnaissance against a single target.",
		Body: `// Recon mission template — produced by gibson mission new --from-template recon.
//
// Discover the target's exposed surface with the tools that ship in
// gibson-executor: subdomains (subfinder), the addresses they resolve to
// (dnsx), live HTTP services (httpx), and open ports (naabu). Four tool
// nodes run in sequence.

_target: "example.com" // set the root domain every tool node scans

mission: {
    name:        "recon"
    description: "Reconnaissance across a target's exposed surface."
    version:     "1.0.0"
    target_ref:  "" // set the target name or ID before submitting

    nodes: {
        subdomains: {
            id:   "subdomains"
            type: "NODE_TYPE_TOOL"
            tool_config: {
                tool_name: "subfinder"
                input: target: _target
            }
        }
        resolve: {
            id:   "resolve"
            type: "NODE_TYPE_TOOL"
            tool_config: {
                tool_name: "dnsx"
                input: target: _target
            }
        }
        http: {
            id:   "http"
            type: "NODE_TYPE_TOOL"
            tool_config: {
                tool_name: "httpx"
                input: target: _target
            }
        }
        ports: {
            id:   "ports"
            type: "NODE_TYPE_TOOL"
            tool_config: {
                tool_name: "naabu"
                input: {
                    target: _target
                    ports:  "21,22,25,53,80,110,143,443,465,587,993,995,3306,3389,5432,6379,8080,8443"
                }
            }
        }
    }
    edges: [
        {from: "subdomains", to: "resolve"},
        {from: "resolve", to: "http"},
        {from: "http", to: "ports"},
    ]
    entry_points: ["subdomains"]
    exit_points:  ["ports"]
}
`,
	},
	"webapp-scan": {
		Synopsis: "Web application discovery + vulnerability scan.",
		Body: `// Web app scan template — gibson mission new --from-template webapp-scan.
mission: {
    name:        "webapp-scan"
    description: "Crawl + active scan a web application."
    version:     "1.0.0"
    target_ref:  ""
    nodes: {
        crawl: {
            id:   "crawl"
            type: "NODE_TYPE_AGENT"
            agent_config: { agent_name: "webcrawl-agent" }
        }
        scan: {
            id:   "scan"
            type: "NODE_TYPE_AGENT"
            agent_config: { agent_name: "webvuln-agent" }
        }
    }
    edges: [{from: "crawl", to: "scan"}]
    entry_points: ["crawl"]
    exit_points:  ["scan"]
}
`,
	},
	"secrets-audit": {
		Synopsis: "Repository secrets audit (gitleaks-style).",
		Body: `// Secrets audit template — gibson mission new --from-template secrets-audit.
mission: {
    name:        "secrets-audit"
    description: "Scan a repository for committed secrets."
    version:     "1.0.0"
    target_ref:  ""
    nodes: {
        leaks: {
            id:   "leaks"
            type: "NODE_TYPE_AGENT"
            agent_config: { agent_name: "gitleaks-agent" }
        }
    }
    entry_points: ["leaks"]
    exit_points:  ["leaks"]
}
`,
	},
	"scan-fix-verify": {
		Synopsis: "Scan a target, fix the findings on a bank, verify the fix.",
		Body: `// Scan, fix, verify template — gibson mission new --from-template scan-fix-verify.
//
// A scanner finds what is wrong. A job on a bank of always-on coding
// agents fixes it in a worktree. A verifier judges the fix. The job node
// sends the report back to the same job until it passes or the passes
// run out, then opens the merge request.
//
// Override before submitting: target_ref, bank_ref, and the repository
// connector_ref and project.
mission: {
    name:        "scan-fix-verify"
    description: "Scan a target, fix the findings on a bank, verify the fix."
    version:     "1.0.0"
    target_ref:  ""
    nodes: {
        scan: {
            id:   "scan"
            type: "NODE_TYPE_AGENT"
            agent_config: { agent_name: "webvuln-agent" }
        }
        fix: {
            id:      "fix"
            type:    "NODE_TYPE_JOB"
            timeout: "5400s"
            job_config: {
                bank_ref: "FIXME-bank"
                spec: {
                    goal: "Fix every finding the scan node reported. Add a regression test for each fix."
                    repositories: [{
                        name:          "app"
                        connector_ref: "connector/gitlab"
                        project:       "group/repo"
                        base_branch:   "main"
                        deliverable:   "DELIVERABLE_KIND_MERGE_REQUEST"
                    }]
                    inputs: ["scan"]
                    acceptance: {
                        verifier_component: "agent/webvuln-agent"
                        passing_score:      0.8
                        max_passes:         3
                    }
                }
                constraints: { max_turns: 40 }
            }
        }
    }
    edges: [{from: "scan", to: "fix"}]
    entry_points: ["scan"]
    exit_points:  ["fix"]
}
`,
	},
	"compliance-check": {
		Synopsis: "Cloud-config compliance check against a baseline policy.",
		Body: `// Compliance check template — gibson mission new --from-template compliance-check.
mission: {
    name:        "compliance-check"
    description: "Audit cloud configuration against a policy baseline."
    version:     "1.0.0"
    target_ref:  ""
    nodes: {
        inspect: {
            id:   "inspect"
            type: "NODE_TYPE_AGENT"
            agent_config: { agent_name: "compliance-agent" }
        }
    }
    entry_points: ["inspect"]
    exit_points:  ["inspect"]
}
`,
	},
}

type builtinTemplate struct {
	Synopsis string
	Body     string
}

const minimalScaffold = `// Minimal mission scaffold — gibson mission new.
//
// Replace FIXME values (target_ref must be a target UUID), then validate with:
//   gibson mission validate this-file.cue
//
// And submit with:
//   gibson mission submit this-file.cue

mission: {
    name:        "FIXME-mission-name"
    description: "FIXME: short description"
    version:     "0.1.0"
    // FIXME: a target UUID, from "gibson target create" or "gibson target list".
    target_ref:  "FIXME-target-ref"

    nodes: {
        step1: {
            id:   "step1"
            type: "NODE_TYPE_AGENT"
            agent_config: {
                agent_name: "FIXME-agent-name"
            }
        }
    }
    entry_points: ["step1"]
    exit_points:  ["step1"]
}
`

func newCmd() *cobra.Command {
	var (
		fromTemplate string
		listTpls     bool
		outPath      string
		targetRef    string
		noTarget     bool
		gibsonURL    string
		timeout      time.Duration
	)
	c := &cobra.Command{
		Use:   "new",
		Short: "Scaffold a new mission file",
		Long: `Scaffold a new mission file.

With --from-template <name>, writes the named template's content. Use
--list-templates to see available templates. Without flags, writes a
minimal scaffold with FIXME placeholders.

The scaffold names a target, so what it writes submits as written. Pass
--target <name-or-uuid> to choose one; with no --target the command reads
your tenant's targets and uses the only one, or asks you to pick when there
is more than one. A UUID is taken as given and needs no daemon.

Pass --no-target to scaffold offline. The file then carries an empty
target_ref, and submit refuses it until you fill it in or pass --target.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if listTpls {
				names := make([]string, 0, len(builtinTemplates))
				for n := range builtinTemplates {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, n := range names {
					fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", n, builtinTemplates[n].Synopsis)
				}
				return nil
			}

			var body string
			if fromTemplate != "" {
				tpl, ok := builtinTemplates[fromTemplate]
				if !ok {
					names := make([]string, 0, len(builtinTemplates))
					for n := range builtinTemplates {
						names = append(names, n)
					}
					sort.Strings(names)
					return fmt.Errorf("template %q not found; available: %v", fromTemplate, names)
				}
				body = tpl.Body
			} else {
				body = minimalScaffold
			}

			if noTarget {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"scaffolded with an empty target_ref: fill it in, or pass --target <name-or-uuid> to submit")
			} else {
				ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
				defer cancel()
				id, name, err := resolveTarget(ctx, gibsonURL, targetRef)
				if err != nil {
					return err
				}
				body = withTarget(body, id)
				if name != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "target: %s (%s)\n", name, id)
				}
			}

			if outPath == "" || outPath == "-" {
				_, err := fmt.Fprint(cmd.OutOrStdout(), body)
				return err
			}
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
			return os.WriteFile(outPath, []byte(body), 0o644)
		},
	}
	c.Flags().StringVar(&fromTemplate, "from-template", "", "Name of a built-in template to scaffold from")
	c.Flags().BoolVar(&listTpls, "list-templates", false, "List available templates and exit")
	c.Flags().StringVarP(&outPath, "output", "o", "-", "Output path; '-' for stdout")
	c.Flags().StringVar(&targetRef, "target", "", "Target name or UUID to write into the scaffold")
	c.Flags().BoolVar(&noTarget, "no-target", false, "Scaffold offline, leaving target_ref empty")
	c.Flags().StringVar(&gibsonURL, "gibson-url", "", "Daemon URL (default: your login session's)")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for the target lookup")
	c.MarkFlagsMutuallyExclusive("target", "no-target")
	return c
}
