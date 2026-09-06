// Stub package for gibson.job.v1.
//
// The mission schema's generated CUE imports this package as:
//
//   jobpb "github.com/zeroroot-ai/sdk/api/proto/gibson/job/v1"
//
// (scripts/regen-cue.sh rewrites the raw `api/gen/...:jobpb` colon import
// into that form.) A template imports it at the same path, for example
//
//   import jobv1 "github.com/zeroroot-ai/sdk/api/proto/gibson/job/v1"
//
// to name a DeliverableKind. The real definitions live at that path in the
// SDK. That package also carries JobService, the job record, and the event
// stream.
// The mission schema needs none of it. #JobNodeConfig references #JobSpec
// only, so this stub carries #JobSpec and the two messages it nests.
//
// Mirrors api/proto/gibson/types/v1/types_stub.cue: scalar fields are typed
// concretely, and the TypedValue map is opened (any) so the stub compiles
// without pulling in commonpb.
//
// Drift guard: TestCUESchemaValidation_JobNode runs a job-node mission
// through this stub and then through protojson into the SDK Go types. A
// renamed or retyped field in job.proto fails that test.
//
// Embedded into the adk CLI binary via //go:embed in schema.go.
package v1

// #DeliverableKind is what the member does with the work on a repository
// at wrap-up.
#DeliverableKind:
	#DELIVERABLE_KIND_UNSPECIFIED |
	#DELIVERABLE_KIND_PUSH_BRANCH |
	#DELIVERABLE_KIND_MERGE_REQUEST |
	#DELIVERABLE_KIND_NONE

#DELIVERABLE_KIND_UNSPECIFIED: 0

// DELIVERABLE_KIND_PUSH_BRANCH: push the job branch to the remote.
#DELIVERABLE_KIND_PUSH_BRANCH: 1

// DELIVERABLE_KIND_MERGE_REQUEST: push the job branch and open a merge
// request against base_branch.
#DELIVERABLE_KIND_MERGE_REQUEST: 2

// DELIVERABLE_KIND_NONE: leave the work in the sandbox. Nothing leaves.
#DELIVERABLE_KIND_NONE: 3

#DeliverableKind_value: {
	DELIVERABLE_KIND_UNSPECIFIED:   0
	DELIVERABLE_KIND_PUSH_BRANCH:   1
	DELIVERABLE_KIND_MERGE_REQUEST: 2
	DELIVERABLE_KIND_NONE:          3
}

// #RepositorySpec names one repository a job works in and what leaves the
// sandbox at wrap-up.
#RepositorySpec: {
	// name is the directory name of the worktree the member sees.
	name?: string

	// connectorRef names the connector that holds the forge credential, in
	// the slash form "connector/<name>".
	connectorRef?: string

	// project is the project path on the forge, for example "group/repo".
	project?: string

	// baseBranch is the branch the job branch starts from and the merge
	// request targets. Empty means the project default branch.
	baseBranch?: string

	// deliverable is what the member does with the work at wrap-up.
	deliverable?: #DeliverableKind
}

// #Acceptance is how a job is judged. The job node executor runs the
// verify loop against it.
#Acceptance: {
	// verifierComponent names the component that judges the work, in the
	// slash form "agent/<name>". Empty means no verify loop.
	verifierComponent?: string

	// passingScore is the score, from 0.0 to 1.0, at or above which the
	// verifier accepts the work.
	passingScore?: float & >=0.0 & <=1.0

	// maxPasses is how many verify passes the job node executor runs before
	// it closes the job as FAILED. Zero means one pass. This bounds the
	// verify loop INSIDE one job. A mission node's RetryPolicy is a
	// different thing: it retries the whole node.
	maxPasses?: int32 & >=0
}

// #JobSpec is the structured input that opens a job.
//
// A JobSpec says WHAT to do. It carries no execution bounds, by design.
// Bounds are gibson.types.v1.TaskConstraints. They ride on
// #JobNodeConfig.constraints and on gibson.types.v1.Task.constraints.
#JobSpec: {
	// goal is what the job must achieve, in plain words.
	goal?: string

	// repositories lists the repositories the job works in.
	repositories?: [...#RepositorySpec]

	// credentialNames lists the credentials the member may read during the
	// job. The per-turn grant covers these and no others.
	credentialNames?: [...string]

	// inputs lists World node ids the job starts from: findings, plans,
	// earlier deliverables.
	inputs?: [...string]

	// acceptance is how the job is judged.
	acceptance?: #Acceptance

	// context is free-form context for the member. The daemon does not read
	// it. Opened (any) so the stub does not pull in commonpb.
	context?: {
		[string]: _
	}
}
