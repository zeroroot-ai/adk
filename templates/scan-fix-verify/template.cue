// Scan, fix, verify.
//
// A scanner finds what is wrong. A job on a bank of always-on Claude Code
// members fixes it in a worktree. A verifier judges the fix; the job node
// sends the report back to the same job until it passes or the passes run
// out, then opens the merge request.
//
// Override before submitting:
//   targetRef: "<target-name-or-id>"
//   bankRef:   "<bank name or id>" (FIXME-bank fails at submit on purpose)
//   the repository connectorRef and project
//
// Spec: gibson#1706 (ADR-0019), epic decisions 15 and 18.

import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"
import jobv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/job/v1"

mission: missionv1.#MissionDefinition & {
	name:        "scan-fix-verify"
	description: "Scan a target, fix the findings on a bank, verify the fix."
	version:     "1.0.0"
	targetRef:   ""

	nodes: {
		scan: {
			id:   "scan"
			type: missionv1.#NODE_TYPE_AGENT
			agentConfig: {
				agentName: "webvuln-agent"
			}
		}
		fix: {
			id:   "fix"
			type: missionv1.#NODE_TYPE_JOB
			timeout: "5400s"
			jobConfig: {
				bankRef: "FIXME-bank"
				spec: {
					goal: "Fix every finding the scan node reported. Add a regression test for each fix."
					repositories: [
						{
							name:         "app"
							connectorRef: "connector/gitlab"
							project:      "group/repo"
							baseBranch:   "main"
							deliverable:  jobv1.#DELIVERABLE_KIND_MERGE_REQUEST
						},
					]
					inputs: ["scan"]
					acceptance: {
						verifierComponent: "agent/webvuln-agent"
						passingScore:      0.8
						maxPasses:         3
					}
				}
				constraints: {
					maxTurns: 40
				}
			}
		}
	}
	edges: [
		{from: "scan", to: "fix"},
	]
	entryPoints: ["scan"]
	exitPoints: ["fix"]
}
