// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
)

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
// Replace FIXME values, then validate with:
//   gibson mission validate this-file.cue
//
// And submit with:
//   gibson mission submit this-file.cue

mission: {
    name:        "FIXME-mission-name"
    description: "FIXME: short description"
    version:     "0.1.0"
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
	)
	c := &cobra.Command{
		Use:   "new",
		Short: "Scaffold a new mission file",
		Long: `Scaffold a new mission file.

With --from-template <name>, writes the named template's content. Use
--list-templates to see available templates. Without flags, writes a
minimal scaffold with FIXME placeholders.`,
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
	return c
}
