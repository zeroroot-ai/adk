// Recon mission template.
//
// Discover the target's exposed surface with the tools that ship in
// gibson-executor: subdomains (subfinder), the addresses they resolve
// to (dnsx), live HTTP services (httpx), and open ports (naabu). Four
// tool nodes run in sequence and land Domain, Subdomain, Host, Port,
// and Service nodes in the knowledge graph.
//
// Override before submitting:
//   targetRef: "<target-name-or-id>"
//   _target:   "<root-domain>"  (the value every tool node scans)
//
// Spec: mission-authoring-cue Requirement 7.

import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

_target: "example.com"

mission: missionv1.#MissionDefinition & {
	name:        "recon"
	description: "Reconnaissance across a target's exposed surface."
	version:     "1.0.0"
	targetRef:   ""

	nodes: {
		subdomains: {
			id:   "subdomains"
			type: missionv1.#NODE_TYPE_TOOL
			toolConfig: {
				toolName: "subfinder"
				input: target: _target
			}
		}
		resolve: {
			id:   "resolve"
			type: missionv1.#NODE_TYPE_TOOL
			toolConfig: {
				toolName: "dnsx"
				input: target: _target
			}
		}
		http: {
			id:   "http"
			type: missionv1.#NODE_TYPE_TOOL
			toolConfig: {
				toolName: "httpx"
				input: target: _target
			}
		}
		ports: {
			id:   "ports"
			type: missionv1.#NODE_TYPE_TOOL
			toolConfig: {
				toolName: "naabu"
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
	entryPoints: ["subdomains"]
	exitPoints: ["ports"]
}
