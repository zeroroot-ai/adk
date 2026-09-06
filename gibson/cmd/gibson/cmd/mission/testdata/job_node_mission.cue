// Fixture: a mission that drives a bank of always-on coding agents.
//
// This is the shape zeroroot-ai/gibson#1706 introduced. It exercises every
// part of the job path through the embedded schema bundle:
//
//   NODE_TYPE_JOB          the node type added in sdk v0.177.0
//   JobNodeConfig.bankRef  the bank the executor opens the job on
//   JobNodeConfig.spec     the jobpb.JobSpec stub
//   JobNodeConfig.constraints  the bounds, which ride on the node, never
//                              on the JobSpec
//   Acceptance             verifierComponent, passingScore, maxPasses
//
// TestCUESchemaValidation_JobNode runs this file through CUE and then
// through protojson into the SDK Go types. A rename or a retype in the SDK
// proto fails that test here, not at a user's `gibson mission submit`.

import (
	missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"
	jobv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/job/v1"
)

mission: missionv1.#MissionDefinition & {
	name:        "ledger-reconciliation"
	description: "Drive a bank of coding agents to reconcile the ledger service."
	version:     "1.0.0"
	nodes: {
		reconcile: {
			id:   "reconcile"
			type: missionv1.#NODE_TYPE_JOB
			jobConfig: {
				bankRef: "bank/core-banking"
				spec: {
					goal: "Reconcile the ledger service against the settlement report."
					repositories: [{
						name:         "ledger"
						connectorRef: "connector/gitlab-core"
						project:      "bank/ledger"
						baseBranch:   "main"
						deliverable:  jobv1.#DELIVERABLE_KIND_MERGE_REQUEST
					}]
					credentialNames: ["gitlab-core-token"]
					inputs: ["world/finding/settlement-drift"]
					acceptance: {
						verifierComponent: "agent/ledger-verifier"
						passingScore:      0.9
						maxPasses:         3
					}
				}
				constraints: {
					maxTurns:  40
					maxTokens: 400000
				}
			}
		}
	}
	entryPoints: ["reconcile"]
	exitPoints: ["reconcile"]
}
