// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jobv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/job/v1"
	missionv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/mission/v1"
)

// TestCUESchemaValidation_Positive verifies that all four shipped
// templates parse and validate cleanly through the schema-aware path.
// Failure here means either the embedded schema bundle is broken or a
// template was accidentally drifted out of spec.
func TestCUESchemaValidation_Positive(t *testing.T) {
	templates := []struct {
		name string
		src  string
	}{
		{
			name: "recon",
			src: `import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

mission: missionv1.#MissionDefinition & {
	name:        "recon"
	description: "Reconnaissance across a target's exposed surface."
	version:     "1.0.0"
	targetRef:   ""
	nodes: {
		scan: {
			id:   "scan"
			type: missionv1.#NODE_TYPE_AGENT
			agentConfig: { agentName: "nmap-agent" }
		}
	}
	entryPoints: ["scan"]
	exitPoints: ["scan"]
}
`,
		},
		{
			name: "webapp-scan",
			src: `import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

mission: missionv1.#MissionDefinition & {
	name:    "webapp-scan"
	version: "1.0.0"
	nodes: {
		scan: {
			id:   "scan"
			type: missionv1.#NODE_TYPE_AGENT
			agentConfig: { agentName: "webvuln-agent" }
		}
	}
	entryPoints: ["scan"]
	exitPoints: ["scan"]
}
`,
		},
		{
			name: "no-import-inline",
			src: `mission: {
	name:    "inline"
	version: "1.0.0"
}
`,
		},
	}

	for _, tc := range templates {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMission([]byte(tc.src), "cue")
			if err != nil {
				t.Fatalf("expected ok, got: %v", err)
			}
		})
	}
}

// TestCUESchemaValidation_Negative proves that a structurally invalid
// template is rejected at CUE-evaluation time — before protojson.Unmarshal
// ever runs. Three distinct structural mistakes are tested.
//
// Each sub-test asserts:
//  1. The error message starts with "cue build:" (CUE layer, not proto layer).
//  2. The error message does NOT contain "proto" (confirming the proto step
//     was never reached).
//  3. The error mentions the offending field path.
func TestCUESchemaValidation_Negative(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantInErr   string // must appear in error string
		wantPrefix  string // error must start with this
		mustNotHave string // must NOT appear (guards against proto-layer slip-through)
	}{
		{
			name: "unknown_top_level_field",
			src: `import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

mission: missionv1.#MissionDefinition & {
	name:        "broken"
	bogus_field: "not in schema"
}
`,
			wantInErr:   "bogus_field",
			wantPrefix:  "cue build:",
			mustNotHave: "proto",
		},
		{
			name: "wrong_type_for_nodes",
			src: `import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

mission: missionv1.#MissionDefinition & {
	name:  "broken"
	nodes: "not-a-struct"
}
`,
			wantInErr:   "nodes",
			wantPrefix:  "cue build:",
			mustNotHave: "proto",
		},
		{
			// #MissionNode has a disjunction of config oneofs, so an unknown
			// field inside a node causes CUE to report "empty disjunction"
			// at the node path rather than "field not allowed" for the unknown
			// field itself. The assertion checks the full field path is present
			// in the error, confirming this is still a CUE-layer rejection.
			name: "unknown_node_field",
			src: `import missionv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"

mission: missionv1.#MissionDefinition & {
	name: "broken"
	nodes: {
		bad: {
			id:          "bad"
			type:        missionv1.#NODE_TYPE_AGENT
			agentConfig: { agentName: "x" }
			not_a_field: "disallowed"
		}
	}
	entryPoints: ["bad"]
	exitPoints:  ["bad"]
}
`,
			// CUE surfaces the disjunction failure at the node path.
			wantInErr:   "mission.nodes.bad",
			wantPrefix:  "cue build:",
			mustNotHave: "proto",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMission([]byte(tc.src), "cue")
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			got := err.Error()
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("error must start with %q\ngot: %s", tc.wantPrefix, got)
			}
			if !strings.Contains(got, tc.wantInErr) {
				t.Errorf("error must mention %q\ngot: %s", tc.wantInErr, got)
			}
			if strings.Contains(got, tc.mustNotHave) {
				t.Errorf("error must NOT contain %q (indicates proto layer ran before CUE)\ngot: %s", tc.mustNotHave, got)
			}
		})
	}
}

// TestCUESchemaValidation_JobNode proves the embedded schema accepts a
// mission that drives a bank (zeroroot-ai/gibson#1706, sdk v0.177.0).
//
// The fixture goes through the whole authoring path: CUE build against the
// embedded bundle, then protojson into the SDK Go types. That makes the
// test a drift guard for the hand-maintained jobpb stub. A renamed or
// retyped field in the SDK job proto fails here, not at a user's
// `gibson mission submit`.
func TestCUESchemaValidation_JobNode(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "job_node_mission.cue"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	def, err := parseMission(src, "cue")
	if err != nil {
		t.Fatalf("expected the job-node fixture to validate, got: %v", err)
	}

	node, ok := def.GetNodes()["reconcile"]
	if !ok {
		t.Fatalf("node %q missing from %v", "reconcile", def.GetNodes())
	}
	if got := node.GetType(); got != missionv1.NodeType_NODE_TYPE_JOB {
		t.Fatalf("node type: got %v, want NODE_TYPE_JOB", got)
	}

	cfg := node.GetJobConfig()
	if cfg == nil {
		t.Fatal("job_config is nil; the oneof did not decode")
	}
	if got := cfg.GetBankRef(); got != "bank/core-banking" {
		t.Fatalf("bank_ref: got %q", got)
	}
	// Bounds ride on the node, never on the JobSpec.
	if got := cfg.GetConstraints().GetMaxTurns(); got != 40 {
		t.Fatalf("constraints.max_turns: got %d, want 40", got)
	}

	spec := cfg.GetSpec()
	if spec == nil {
		t.Fatal("job spec is nil")
	}
	if len(spec.GetRepositories()) != 1 {
		t.Fatalf("repositories: got %d, want 1", len(spec.GetRepositories()))
	}
	repo := spec.GetRepositories()[0]
	if got := repo.GetConnectorRef(); got != "connector/gitlab-core" {
		t.Fatalf("repository.connector_ref: got %q", got)
	}
	if got := repo.GetDeliverable(); got != jobv1.DeliverableKind_DELIVERABLE_KIND_MERGE_REQUEST {
		t.Fatalf("repository.deliverable: got %v, want MERGE_REQUEST", got)
	}
	if got := spec.GetCredentialNames(); len(got) != 1 || got[0] != "gitlab-core-token" {
		t.Fatalf("credential_names: got %v", got)
	}
	if got := spec.GetInputs(); len(got) != 1 || got[0] != "world/finding/settlement-drift" {
		t.Fatalf("inputs: got %v", got)
	}

	acc := spec.GetAcceptance()
	if acc == nil {
		t.Fatal("acceptance is nil")
	}
	if got := acc.GetVerifierComponent(); got != "agent/ledger-verifier" {
		t.Fatalf("acceptance.verifier_component: got %q", got)
	}
	if got := acc.GetPassingScore(); got != 0.9 {
		t.Fatalf("acceptance.passing_score: got %v, want 0.9", got)
	}
	// max_passes bounds the verify loop inside one job. RetryPolicy is a
	// different thing: it retries the whole node.
	if got := acc.GetMaxPasses(); got != 3 {
		t.Fatalf("acceptance.max_passes: got %d, want 3", got)
	}
}

// TestCUESchemaValidation_ShippedTemplates runs every shipped template
// under templates/<name>/template.cue through the schema-aware path, the
// same path `make templates-vet` drives through the CLI. It also pins the
// one template the dashboard vendors for the job node round trip
// (zeroroot-ai/dashboard#1171): scan-fix-verify must carry a job node
// whose acceptance names a verifier.
func TestCUESchemaValidation_ShippedTemplates(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "..", "templates", "*", "template.cue"))
	if err != nil {
		t.Fatalf("glob templates: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no templates found under templates/*/template.cue")
	}

	// parsed keeps every template that validated, keyed by directory name,
	// so the pin below reads the definition the subtest already checked.
	parsed := map[string]*missionv1.MissionDefinition{}
	for _, p := range paths {
		name := filepath.Base(filepath.Dir(p))
		t.Run(name, func(t *testing.T) {
			// The glob is anchored to the repo's templates/ directory, so
			// filepath.Clean is what gosec G304 asks for and nothing more.
			src, err := os.ReadFile(filepath.Clean(p))
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			def, err := parseMission(src, "cue")
			if err != nil {
				t.Fatalf("expected %s to validate, got: %v", p, err)
			}
			if got := def.GetName(); got != name {
				t.Fatalf("template %s declares name %q; the directory name and the mission name must match", p, got)
			}
			parsed[name] = def
		})
	}

	def, ok := parsed["scan-fix-verify"]
	if !ok {
		t.Fatal("templates/scan-fix-verify/template.cue is missing or did not validate; the dashboard vendors it (dashboard#1171)")
	}
	fix := def.GetNodes()["fix"]
	if fix.GetType() != missionv1.NodeType_NODE_TYPE_JOB {
		t.Fatalf("scan-fix-verify node fix: got %v, want NODE_TYPE_JOB", fix.GetType())
	}
	acc := fix.GetJobConfig().GetSpec().GetAcceptance()
	if acc.GetVerifierComponent() == "" || acc.GetMaxPasses() == 0 {
		t.Fatalf("scan-fix-verify acceptance must name a verifier and bound the loop, got %v", acc)
	}
	if got := fix.GetJobConfig().GetSpec().GetRepositories()[0].GetDeliverable(); got != jobv1.DeliverableKind_DELIVERABLE_KIND_MERGE_REQUEST {
		t.Fatalf("scan-fix-verify deliverable: got %v, want MERGE_REQUEST", got)
	}
}
