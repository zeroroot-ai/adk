// Fixture: a mission that declares which named tenant secrets its components
// may be handed at dispatch (zeroroot-ai/gibson#485, sdk v0.191.0).
//
// It exercises every scope the block has, because the scopes UNION rather than
// narrow and a mission author has to be able to read that off the file:
//
//   mission      every component in the run, whatever its kind
//   agents       every agent
//   tools        every tool
//   plugins      every plugin
//   agent[name]  one named agent
//   tool[name]   one named tool
//   plugin[name] one named plugin
//
// NAMES ONLY. The value is resolved server-side at dispatch and handed to the
// component in its environment. A value never appears in a mission definition,
// because a definition is stored, listed, rendered, validated and displayed.
//
// The tool node passes the secret's NAME in its input for the same reason: a
// tool's input JSON is captured with the tool call.
//
// TestCUESchemaValidation_MissionSecrets runs this file through CUE and then
// through protojson into the SDK Go types, so a rename or a retype in the SDK
// mission proto fails there rather than at a user's `gibson mission submit`.

import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

mission: missionv1.#MissionDefinition & {
	name:        "cluster-assessment-fixture"
	description: "Assess a cluster with a tool that is handed a kubeconfig the mission declared."
	version:     "1.0.0"

	secrets: {
		mission: ["cred:tenant-ca"]
		agents: ["cred:agent-wide"]
		tools: ["cred:goat-cluster"]
		plugins: ["cred:plugin-wide"]
		agent: "zerocool": names: ["cred:zerocool-only"]
		tool: "kube-bench": names: ["cred:kube-bench-only"]
		plugin: "github-plugin": names: ["cred:github-token"]
	}

	nodes: {
		benchmark: {
			id:   "benchmark"
			type: missionv1.#NODE_TYPE_TOOL
			toolConfig: {
				toolName: "kube-bench"
				input: {
					target:           "{{target.name}}"
					kubeconfigSecret: "cred:goat-cluster"
				}
			}
		}
	}
	entryPoints: ["benchmark"]
	exitPoints: ["benchmark"]
}
