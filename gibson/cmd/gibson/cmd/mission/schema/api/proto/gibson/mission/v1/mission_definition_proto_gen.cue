// Schema evolution policy (mission-schema-canonicalization Requirement 7):
//
//   1. NodeType, Language, BackoffStrategy, and any future enum values
//      are append-only. Existing values are NEVER renumbered or removed.
//   2. Deprecated values are marked `[deprecated = true]` and accompanied
//      by a `// reserved` comment explaining the supersession.
//   3. MissionNode.config oneof variants are append-only with the same
//      discipline; reuse of a tag number is forbidden.
//   4. Adding a new node type or expression language requires the
//      controlled-extension contract defined in
//      `mission-verb-noun-registry`: a matching *NodeConfig message in
//      the oneof, a registered handler in the daemon, and a conformance
//      test exercising the new type end-to-end. CI enforces all four.
//
// The proto under this package is the single source of truth for the
// mission schema. The daemon, the `gibson` CLI (ADK), and the
// dashboard all consume the generated bindings of this file.
// Hand-written parallel representations are forbidden.

// NOTE: This copy is embedded in the adk CLI binary for offline schema
// validation. It is regenerated from the SDK proto by
// scripts/regen-cue.sh — DO NOT EDIT BY HAND. The freshness gate
// (scripts/check-cue-fresh.sh) fails CI if this file drifts from the
// SDK proto. See zeroroot-ai/adk#27.
package v1

import (
	"time"
	typespb "github.com/zeroroot-ai/sdk/api/proto/gibson/types/v1"
	jobpb "github.com/zeroroot-ai/sdk/api/proto/gibson/job/v1"
)

// MissionDefinition represents a mission template/definition.
// This is the shareable mission specification that can be created via the
// CreateMissionDefinition API and referenced by mission runs.
#MissionDefinition: {
	// ID is the unique identifier for this mission definition
	id?: string @protobuf(1,string)

	// Name is a human-readable name for the mission
	name?: string @protobuf(2,string)

	// Description provides additional context about what this mission does
	description?: string @protobuf(3,string)

	// Version is the semantic version of the mission definition
	version?: string @protobuf(4,string)

	// TargetRef is a reference to the target (name or ID)
	// This needs to be resolved to a TargetID when creating a mission instance
	targetRef?: string @protobuf(5,string,name=target_ref)

	// Nodes contains all the nodes in the mission, indexed by node ID
	nodes?: {
		[string]: #MissionNode
	} @protobuf(6,map[string]MissionNode)

	// Edges contains all the directed edges connecting nodes in the mission
	edges?: [...#MissionEdge] @protobuf(7,MissionEdge)

	// EntryPoints contains the IDs of nodes that can serve as entry points to the mission
	// These are nodes with no incoming edges
	entryPoints?: [...string] @protobuf(8,string,name=entry_points)

	// ExitPoints contains the IDs of nodes that can serve as exit points from the mission
	// These are nodes with no outgoing edges
	exitPoints?: [...string] @protobuf(9,string,name=exit_points)

	// Metadata contains additional custom metadata for the mission
	metadata?: {
		[string]: string
	} @protobuf(10,map[string]string)

	// Dependencies specifies required agents and tools for this mission
	dependencies?: #MissionDependencies @protobuf(11,MissionDependencies)

	// Source is the git URL this mission was installed from (if applicable)
	source?: string @protobuf(12,string)

	// InstalledAt is the timestamp when this mission was installed
	installedAt?: time.Time @protobuf(13,google.protobuf.Timestamp,name=installed_at)

	// CreatedAt is the timestamp when the mission definition was created
	createdAt?: time.Time @protobuf(14,google.protobuf.Timestamp,name=created_at)

	// Workspace configures repository cloning + workspace
	// management for agents that need code access. Optional;
	// missions without code interaction omit this field.
	//
	// Spec: mission-schema-canonicalization (mirror migration).
	workspace?: #WorkspaceConfig @protobuf(15,WorkspaceConfig)

	// Constraints declares mission-level operational limits. When present,
	// these are the authoritative constraints for the mission DAG — they
	// make the mission self-describing so authors do not have to supply limits
	// out-of-band at dispatch time. The daemon merges these with any
	// dispatch-time overrides (dispatch wins on conflict).
	// Optional: absent means no constraints are baked into the definition.
	//
	// Spec: sdk#47 (MissionConstraints proto promotion).
	constraints?: #MissionConstraints @protobuf(16,MissionConstraints)

	// DeciderSlot names the mission-level LLM the brain's Decider runs on — the
	// orchestration decision-maker, distinct from per-node agent slots
	// (gibson#850). Only the provider+model are used (the slot name is implicitly
	// "decider"). Absent means the brain uses the tenant's dashboard-default
	// provider/model, so missions need not set it.
	deciderSlot?: #LLMSlotConfig @protobuf(17,LLMSlotConfig,name=decider_slot)
}

// MissionConstraints declares the operational limits baked into a mission
// definition. Making constraints part of the schema lets mission authors
// publish self-describing missions (e.g., "stop after 50 findings") without
// requiring callers to supply limits out-of-band at dispatch time.
//
// Semantics (zero means unlimited):
//   - max_duration  : 0 duration  → no time limit
//   - max_tokens    : 0           → no token budget
//   - max_cost      : 0.0         → no cost ceiling
//   - max_findings  : 0           → no finding count limit
//
// Spec: sdk#47 (MissionConstraints proto promotion).
#MissionConstraints: {
	// max_duration is the wall-clock limit for the entire mission.
	// Uses google.protobuf.Duration for sub-second precision.
	// Absent or zero-value means no time limit.
	maxDuration?: time.Duration @protobuf(1,google.protobuf.Duration,name=max_duration)

	// max_tokens is the cumulative LLM token budget across the entire mission
	// (all agent nodes combined). The daemon accumulates usage on every LLM
	// invocation and stops the mission when the budget is exceeded.
	// 0 means unlimited. This is a mission-wide budget, not a per-call limit;
	// use max_tokens_per_call to cap individual invocations.
	maxTokens?: int64 @protobuf(2,int64,name=max_tokens,"(buf.validate.field).int64=")

	// max_cost is the cumulative LLM cost ceiling in USD across all agent
	// nodes. 0.0 means unlimited.
	maxCost?: float64 @protobuf(3,double,name=max_cost,"(buf.validate.field).double=")

	// max_findings is the maximum number of findings to collect before the
	// mission stops. 0 means unlimited.
	maxFindings?: int32 @protobuf(4,int32,name=max_findings,"(buf.validate.field).int32=")

	// severity_threshold is the minimum severity level required to record a
	// finding. Common values: "low", "medium", "high", "critical".
	// Empty string means accept all severities.
	severityThreshold?: string @protobuf(5,string,name=severity_threshold)

	// require_evidence indicates whether all findings must include
	// proof-of-concept evidence before being recorded.
	requireEvidence?: bool @protobuf(6,bool,name=require_evidence)

	// blocked_tools lists tool names that must not be invoked during this
	// mission. The daemon enforces this at dispatch time.
	blockedTools?: [...string] @protobuf(7,string,name=blocked_tools)

	// blocked_domains lists network domains (e.g., "prod.example.com") that
	// agents must not contact. Enforcement is best-effort at the tool level.
	blockedDomains?: [...string] @protobuf(8,string,name=blocked_domains)
	// Fields 9-12 promoted from gibson.daemon.v1.MissionConstraints under
	// ADR 0004 (https://github.com/zeroroot-ai/docs/blob/main/adr/0004-canonical-mission-constraints.md).
	// The daemon-local type is deleted in the same change; this is now the
	// single canonical MissionConstraints for the entire platform.

	// max_turns_per_agent caps the number of agent turns (Observe→Think→Act
	// iterations) for any single agent node in the mission. 0 means unlimited.
	maxTurnsPerAgent?: int32 @protobuf(9,int32,name=max_turns_per_agent,"(buf.validate.field).int32=")

	// allowed_techniques is the allowlist of attack technique IDs (taxonomy)
	// that agents may use during the mission. Empty list means no allowlist
	// (any technique may be used unless blocked).
	allowedTechniques?: [...string] @protobuf(10,string,name=allowed_techniques)

	// blocked_techniques is the blocklist of attack technique IDs that
	// agents must not use, regardless of allowed_techniques. Empty list
	// means no blocklist.
	blockedTechniques?: [...string] @protobuf(11,string,name=blocked_techniques)

	// max_tokens_per_call is the per-invocation cap on LLM tokens for any
	// single LLM call within this mission. Applied by the daemon before every
	// provider call; the provider never sees more than this many output tokens.
	//
	// Precedence cascade (highest → lowest):
	//   1. Per-node *NodeConfig.max_tokens_per_call (when set on a specific node)
	//   2. This field (mission-level default)
	//   3. 0 — no cap from this mechanism
	//
	// When a per-node override is set it completely supersedes this field for
	// that node (including 0, which explicitly disables the cap for that node
	// while this field may still apply to all other nodes).
	//
	// This field is different from max_tokens: max_tokens is a cumulative
	// budget for the entire mission; max_tokens_per_call is a ceiling on each
	// individual LLM call.
	//
	// 0 means unlimited at this level. Spec: mission-schema-canonicalization
	// Requirement 5. Enforced by EffectivePerCallCap() (wired in M4, gibson#133).
	maxTokensPerCall?: int32 @protobuf(12,int32,name=max_tokens_per_call,"(buf.validate.field).int32=")
}

// MissionDependencies specifies required components for a mission
#MissionDependencies: {
	// Agents lists required agent components by name or URL
	agents?: [...string] @protobuf(1,string)

	// Tools lists required tool components by name or URL
	tools?: [...string] @protobuf(2,string)

	// Plugins lists required plugin components by name or URL
	plugins?: [...string] @protobuf(3,string)
}

// NodeType defines the type of mission node
#NodeType:
	#NODE_TYPE_UNSPECIFIED |
	#NODE_TYPE_AGENT |
	#NODE_TYPE_TOOL |
	#NODE_TYPE_PLUGIN |
	#NODE_TYPE_CONDITION |
	#NODE_TYPE_PARALLEL |
	#NODE_TYPE_JOIN |
	#NODE_TYPE_JOB

// Sentinel value - must be first
#NODE_TYPE_UNSPECIFIED: 0

// Agent node executes an agent
#NODE_TYPE_AGENT: 1

// Tool node executes a tool
#NODE_TYPE_TOOL: 2

// Plugin node calls a named method on a multi-method plugin component.
#NODE_TYPE_PLUGIN: 3

// Condition node performs conditional branching
#NODE_TYPE_CONDITION: 4

// Parallel node executes sub-nodes in parallel
#NODE_TYPE_PARALLEL: 5

// Join node waits for multiple branches to complete
#NODE_TYPE_JOIN: 6

// Job node drives a job on a bank of always-on agents (gibson#1706).
// Its executor opens the job, runs the verify loop against the
// acceptance the spec declares, and closes the job with a verdict.
#NODE_TYPE_JOB: 7

#NodeType_value: {
	NODE_TYPE_UNSPECIFIED: 0
	NODE_TYPE_AGENT:       1
	NODE_TYPE_TOOL:        2
	NODE_TYPE_PLUGIN:      3
	NODE_TYPE_CONDITION:   4
	NODE_TYPE_PARALLEL:    5
	NODE_TYPE_JOIN:        6
	NODE_TYPE_JOB:         7
}

// Language declares the expression language used by mission constructs
// that evaluate string expressions (currently only ConditionNodeConfig).
//
// Spec: mission-schema-canonicalization Requirement 4. CEL is the only
// language supported in v1; LANGUAGE_UNSPECIFIED is treated as
// LANGUAGE_CEL for backwards compatibility with documents authored
// before this enum existed.
#Language:
	#LANGUAGE_UNSPECIFIED |
	#LANGUAGE_CEL

// Sentinel value - treated as LANGUAGE_CEL by the daemon.
#LANGUAGE_UNSPECIFIED: 0

// Common Expression Language (cel-spec.dev). The default.
#LANGUAGE_CEL: 1

#Language_value: {
	LANGUAGE_UNSPECIFIED: 0
	LANGUAGE_CEL:         1
}

// MissionNode represents a single node in a mission DAG
#MissionNode: {
	// ID is the unique identifier for this node within the mission
	id?: string @protobuf(1,string)

	// Type is the node type
	type?: #NodeType @protobuf(2,NodeType)

	// Name is a human-readable name for the node
	name?: string @protobuf(3,string)

	// Description provides additional context about this node
	description?: string @protobuf(4,string)
	// Config contains the type-specific configuration (oneof)
	{} | {
		// AgentConfig for agent nodes
		agentConfig: #AgentNodeConfig @protobuf(5,AgentNodeConfig,name=agent_config)
	} | {
		// ToolConfig for tool nodes
		toolConfig: #ToolNodeConfig @protobuf(6,ToolNodeConfig,name=tool_config)
	} | {
		// PluginConfig for plugin nodes
		pluginConfig: #PluginNodeConfig @protobuf(7,PluginNodeConfig,name=plugin_config)
	} | {
		// ConditionConfig for condition nodes
		conditionConfig: #ConditionNodeConfig @protobuf(8,ConditionNodeConfig,name=condition_config)
	} | {
		// ParallelConfig for parallel nodes
		parallelConfig: #ParallelNodeConfig @protobuf(9,ParallelNodeConfig,name=parallel_config)
	} | {
		// JoinConfig for join nodes (mission-verb-noun-registry).
		// Field number 15 because 10-14 are sibling MissionNode fields
		// (dependencies, timeout, retry_policy, data_policy, metadata).
		joinConfig: #JoinNodeConfig @protobuf(15,JoinNodeConfig,name=join_config)
	} | {
		// JobConfig for job nodes (gibson#1706). Field number 17 because
		// 16 is the sibling reuse_policy field.
		jobConfig: #JobNodeConfig @protobuf(17,JobNodeConfig,name=job_config)
	}

	// Dependencies lists node IDs that must complete before this node executes
	dependencies?: [...string] @protobuf(10,string)

	// Timeout is the maximum execution time for this node
	timeout?: time.Duration @protobuf(11,google.protobuf.Duration)

	// RetryPolicy defines retry behavior for this node
	retryPolicy?: #RetryPolicy @protobuf(12,RetryPolicy,name=retry_policy)

	// DataPolicy defines data handling policy for this node.
	//
	// Deprecated: data reuse + scoping are no longer node-declared. Under the
	// ECS brain (gibson#851, ADR-0008) reuse is implicit in the event-sourced
	// World and scoping flows from scope-relative identity (ADR-0002) +
	// ambient projection. The field is retained wire-compatibly for old
	// definitions but is ignored by the engine.
	dataPolicy?: #DataPolicy @protobuf(13,DataPolicy,name=data_policy,deprecated)

	// Metadata contains additional custom metadata for this node
	metadata?: {
		[string]: string
	} @protobuf(14,map[string]string)

	// ReusePolicy declares how this node's I/O is scoped +
	// reused across mission runs.
	//
	// Deprecated: superseded by the ECS brain (gibson#851, ADR-0008). Reuse is
	// implicit in the World; scoping is via scope-relative identity (ADR-0002)
	// + ambient projection. Retained wire-compatibly but ignored by the engine.
	reusePolicy?: #ReusePolicy @protobuf(16,ReusePolicy,name=reuse_policy,deprecated)
}

// AgentNodeConfig contains configuration for agent nodes.
// AGENT = LLM-driven worker that calls tools and plugins on the
// author's behalf. The executor selects an agent component by name
// and dispatches the configured Task.
#AgentNodeConfig: {
	// AgentName is the name of the agent to execute
	agentName?: string @protobuf(1,string,name=agent_name)

	// Task is the agent task configuration
	task?: typespb.#Task @protobuf(2,gibson.types.v1.Task)

	// max_tokens_per_call is the per-node override of
	// MissionConstraints.max_tokens_per_call for this agent node only.
	//
	// When present (non-nil), this value is used as the effective cap for
	// all LLM calls made by this node, regardless of the mission-level
	// MissionConstraints.max_tokens_per_call value. Setting this to 0
	// explicitly disables the cap for this node (the mission-level cap is
	// NOT applied as a fallback when this field is explicitly set to 0).
	//
	// When absent (nil / proto3 optional not set), the mission-level
	// MissionConstraints.max_tokens_per_call applies instead.
	//
	// 0 = inherit from mission-level (when this field is absent).
	// Spec: mission-schema-canonicalization Requirement 5; gibson#133.
	maxTokensPerCall?: int32 @protobuf(3,int32,name=max_tokens_per_call,"(buf.validate.field).int32=")

	// llm_slots pins LLM provider/model bindings per named slot for this agent
	// node. Each entry maps a slot name to a specific provider+model. Multiple
	// entries allow different slots to use different providers/models on the same
	// node. Empty list → the node inherits the tenant's default provider for all
	// slots. Absent slot entries fall through to tenant default + constraint
	// search over the tenant's permitted providers.
	//
	// Empty provider or model within an entry is a valid fall-through marker:
	// the daemon resolves it against the tenant's configured defaults.
	//
	// Precedence at resolution per slot: explicit binding in llm_slots >
	// tenant default > constraint search over the tenant's permitted providers.
	//
	// Spec: sdk#260 (multi-slot LLM binding contract); consumer: gibson#524.
	llmSlots?: [...#LLMSlotConfig] @protobuf(5,LLMSlotConfig,name=llm_slots)
}

// LLMSlotConfig pins the provider/model for an agent node's LLM slot. It is a
// tenant-scoped reference: the provider name must be one the tenant has
// configured (gibson.tenant provider config); model is the provider's model id
// (empty = the provider's default model). Leave the whole message unset to
// inherit the tenant default.
#LLMSlotConfig: {
	// slot is the slot name this binding targets. Empty defaults to "primary".
	slot?: string @protobuf(1,string)

	// provider is the configured provider's name for the calling tenant.
	provider?: string @protobuf(2,string)

	// model is the model id; empty uses the provider's default model.
	model?: string @protobuf(3,string)
}

// ToolNodeConfig contains configuration for tool nodes.
// TOOL = single-purpose named function with a typed input map. The
// executor invokes the named tool through the tool worker queue and
// returns the tool's output as the node result.
#ToolNodeConfig: {
	// ToolName is the name of the tool to execute
	toolName?: string @protobuf(1,string,name=tool_name)

	// Input contains the tool input parameters
	input?: {
		[string]: string
	} @protobuf(2,map[string]string)

	// max_tokens_per_call is the per-node override of
	// MissionConstraints.max_tokens_per_call for this tool node only.
	// Follows the same semantics as AgentNodeConfig.max_tokens_per_call:
	// present and non-zero caps the call; present and 0 disables the cap for
	// this node; absent means fall through to the mission-level constraint.
	// Spec: mission-schema-canonicalization Requirement 5; gibson#133.
	maxTokensPerCall?: int32 @protobuf(3,int32,name=max_tokens_per_call,"(buf.validate.field).int32=")
}

// PluginNodeConfig contains configuration for plugin nodes.
// PLUGIN = multi-method provider keyed by `plugin_name + method`.
// Distinct from TOOL: a plugin advertises several callable methods
// behind one component identity. The executor selects the named
// method and dispatches `params` as the call payload.
#PluginNodeConfig: {
	// PluginName is the name of the plugin to query
	pluginName?: string @protobuf(1,string,name=plugin_name)

	// Method is the plugin method to call
	method?: string @protobuf(2,string)

	// Params contains the method parameters
	params?: {
		[string]: string
	} @protobuf(3,map[string]string)

	// max_tokens_per_call is the per-node override of
	// MissionConstraints.max_tokens_per_call for this plugin node only.
	// Follows the same semantics as AgentNodeConfig.max_tokens_per_call:
	// present and non-zero caps the call; present and 0 disables the cap for
	// this node; absent means fall through to the mission-level constraint.
	// Spec: mission-schema-canonicalization Requirement 5; gibson#133.
	maxTokensPerCall?: int32 @protobuf(4,int32,name=max_tokens_per_call,"(buf.validate.field).int32=")
}

// ConditionNodeConfig contains configuration for condition nodes
#ConditionNodeConfig: {
	// Expression to evaluate (e.g., "result.status == 'success'")
	expression?: string @protobuf(1,string)

	// TrueBranch contains node IDs to execute if condition is true
	trueBranch?: [...string] @protobuf(2,string,name=true_branch)

	// FalseBranch contains node IDs to execute if condition is false
	falseBranch?: [...string] @protobuf(3,string,name=false_branch)

	// Language declares the expression language. Defaults to
	// LANGUAGE_CEL; LANGUAGE_UNSPECIFIED is treated as CEL for
	// backwards compatibility with pre-Language-field documents.
	language?: #Language @protobuf(4,Language)
}

// ParallelNodeConfig contains configuration for parallel nodes.
// PARALLEL fans out to its sub-nodes concurrently, capped by
// max_concurrency. Sibling failures are isolated (one failing
// sub-node does not cancel its siblings). Spec:
// mission-verb-noun-registry Requirement 6.
#ParallelNodeConfig: {
	// SubNodes contains the nodes to execute in parallel
	subNodes?: [...#MissionNode] @protobuf(1,MissionNode,name=sub_nodes)

	// MaxConcurrency limits the number of concurrent executions (0 = unlimited)
	maxConcurrency?: int32 @protobuf(2,int32,name=max_concurrency)
}

// MergeStrategy declares how a JoinNodeConfig combines results
// from its `wait_for` upstream sources.
//
// Spec: mission-verb-noun-registry Requirement 7.
#MergeStrategy:
	#MERGE_STRATEGY_UNSPECIFIED |
	#MERGE_STRATEGY_CONCAT |
	#MERGE_STRATEGY_REDUCE |
	#MERGE_STRATEGY_FIRST |
	#MERGE_STRATEGY_LAST |
	#MERGE_STRATEGY_CUSTOM

// Sentinel - must be first.
#MERGE_STRATEGY_UNSPECIFIED: 0

// CONCAT preserves source order in the merged output.
#MERGE_STRATEGY_CONCAT: 1

// REDUCE applies a built-in reducer (semantics defined in the
// CONDITION/JOIN executor design).
#MERGE_STRATEGY_REDUCE: 2

// FIRST returns the first source to complete.
#MERGE_STRATEGY_FIRST: 3

// LAST returns the last source to complete.
#MERGE_STRATEGY_LAST: 4

// CUSTOM evaluates the JoinNodeConfig.aggregator CEL expression
// against the source results.
#MERGE_STRATEGY_CUSTOM: 5

#MergeStrategy_value: {
	MERGE_STRATEGY_UNSPECIFIED: 0
	MERGE_STRATEGY_CONCAT:      1
	MERGE_STRATEGY_REDUCE:      2
	MERGE_STRATEGY_FIRST:       3
	MERGE_STRATEGY_LAST:        4
	MERGE_STRATEGY_CUSTOM:      5
}

// JoinNodeConfig blocks until every node ID in `wait_for` has
// completed (success or final failure), then merges their results
// per `strategy`. JOIN is a first-class noun separable from
// PARALLEL — a JOIN can merge results from non-parallel branches.
//
// Spec: mission-verb-noun-registry Requirement 7.
#JoinNodeConfig: {
	// wait_for lists the upstream node IDs whose completion is
	// required before this JOIN runs. Must be non-empty;
	// submit-time validation rejects empty wait_for.
	waitFor?: [...string] @protobuf(1,string,name=wait_for,"(buf.validate.field).repeated.min_items=1")

	// strategy selects how upstream results are combined.
	strategy?: #MergeStrategy @protobuf(2,MergeStrategy)

	// aggregator carries a CEL expression used when strategy is
	// MERGE_STRATEGY_CUSTOM. The expression sees `sources` (a map
	// from node ID to that node's result) and returns the merged
	// value. Empty when strategy is not CUSTOM.
	aggregator?: string @protobuf(3,string)
}

// WorkspaceConfig configures repository cloning + workspace
// management for missions whose agents need to interact with
// code. The daemon's workspace manager honors this at mission
// start (initializeWorkspaces).
//
// Spec: mission-schema-canonicalization (mirror migration —
// lifts mission.WorkspaceConfig from the daemon's hand-written
// mirror into the canonical proto schema).
#WorkspaceConfig: {
	// Repositories to clone for this mission. Each entry becomes
	// a workspace addressable from agents via harness.Workspace(name).
	repositories?: [...#RepositoryConfig] @protobuf(1,RepositoryConfig)

	// Settings carries workspace-wide knobs (cleanup, LSP, isolation).
	settings?: #WorkspaceSettings @protobuf(2,WorkspaceSettings)
}

// RepositoryConfig defines a single repository to clone. Maps
// directly to the SDK's workspace.RepositoryConfig type.
#RepositoryConfig: {
	// Name is the unique identifier for this repository within
	// the mission. Required.
	name?: string @protobuf(1,string)

	// URL is the Git repository URL (HTTPS or SSH). Required.
	url?: string @protobuf(2,string)

	// Branch is the Git branch to checkout after cloning.
	// Defaults to the repository's default branch when empty.
	branch?: string @protobuf(3,string)

	// CredentialName references a credential in the credential
	// store. Optional — public repos don't need credentials.
	credentialName?: string @protobuf(4,string,name=credential_name)

	// Shallow enables `git clone --depth 1`.
	shallow?: bool @protobuf(5,bool)

	// DependsOn lists repository names that must clone first.
	// Enables topological ordering for multi-repo missions.
	dependsOn?: [...string] @protobuf(6,string,name=depends_on)
}

// WorkspaceSettings carries workspace-wide options.
#WorkspaceSettings: {
	// CleanupOnComplete deletes workspace directories after the
	// mission ends. Defaults to true at the daemon when unset
	// (cleanup is the safe default).
	cleanupOnComplete?: bool @protobuf(1,bool,name=cleanup_on_complete)

	// UseWorktrees enables Git worktrees for per-agent isolation
	// — concurrent modifications without conflicts.
	useWorktrees?: bool @protobuf(2,bool,name=use_worktrees)

	// LSPEnabled starts language servers for code validation.
	lspEnabled?: bool @protobuf(3,bool,name=lsp_enabled)

	// LSPTimeout caps LSP validation duration. Encoded as a
	// protobuf Duration to preserve sub-second precision.
	lspTimeout?: time.Duration @protobuf(4,google.protobuf.Duration,name=lsp_timeout)

	// BaseDirectory is the workspace clone root. When empty the
	// daemon uses a temp directory.
	baseDirectory?: string @protobuf(5,string,name=base_directory)
}

// ReusePolicy declares how a node's I/O is scoped + reused
// across mission runs.
//
// Deprecated: superseded by the ECS brain (gibson#851, ADR-0008). Reuse is
// implicit in the event-sourced World and scoping flows from scope-relative
// identity (ADR-0002) + ambient projection, so node-declared reuse/scoping no
// longer has meaning. Retained wire-compatibly; ignored by the engine.
#ReusePolicy: {
	@protobuf(option deprecated=true)

	// OutputScope: "mission_run" | "mission" | "global".
	// Default: "mission".
	outputScope?: string @protobuf(1,string,name=output_scope)

	// InputScope: "mission_run" | "mission" | "global".
	// Default: "mission".
	inputScope?: string @protobuf(2,string,name=input_scope)

	// Reuse: "skip" | "rerun" | "merge". Controls behavior when
	// an existing output is found in scope. Default: "rerun".
	reuse?: string @protobuf(3,string)
}

// BackoffStrategy defines the strategy for calculating retry delays
#BackoffStrategy:
	#BACKOFF_STRATEGY_UNSPECIFIED |
	#BACKOFF_STRATEGY_CONSTANT |
	#BACKOFF_STRATEGY_LINEAR |
	#BACKOFF_STRATEGY_EXPONENTIAL

// Sentinel value - must be first
#BACKOFF_STRATEGY_UNSPECIFIED: 0

// Constant returns a constant delay for all retry attempts
#BACKOFF_STRATEGY_CONSTANT: 1

// Linear increases the delay linearly with each retry attempt
#BACKOFF_STRATEGY_LINEAR: 2

// Exponential increases the delay exponentially with each retry attempt
#BACKOFF_STRATEGY_EXPONENTIAL: 3

#BackoffStrategy_value: {
	BACKOFF_STRATEGY_UNSPECIFIED: 0
	BACKOFF_STRATEGY_CONSTANT:    1
	BACKOFF_STRATEGY_LINEAR:      2
	BACKOFF_STRATEGY_EXPONENTIAL: 3
}

// RetryPolicy defines the retry behavior for a mission node
// JobNodeConfig contains configuration for job nodes.
// JOB = a unit of work driven on a bank of always-on coding agents
// (gibson#1706). The executor opens a job on the bank, then runs the verify
// loop: it dispatches the acceptance step to the declared verifier
// component, and on failure sends the verifier's report as the next input
// to the SAME job, so the agent keeps its conversation and its worktrees.
// When the verifier passes, or the passes run out, the executor calls
// CloseJob with a verdict and a score.
//
// The loop lives inside this node. The mission graph stays a DAG: a job
// node never adds a loop edge.
#JobNodeConfig: {
	// bank_ref names the bank to open the job on, by id or by name.
	// Required.
	bankRef?: string @protobuf(1,string,name=bank_ref,"(buf.validate.field).string=")

	// spec is the structured input that opens the job: goal, repositories,
	// credential names, World inputs, and the acceptance the verify loop
	// runs against. Required.
	spec?: jobpb.#JobSpec @protobuf(2,gibson.job.v1.JobSpec,"(buf.validate.field).required")

	// constraints bound every turn of the job. The daemon clamps them to the
	// bank policy. Unset means the bank default.
	constraints?: typespb.#TaskConstraints @protobuf(3,gibson.types.v1.TaskConstraints)
}

#RetryPolicy: {
	// MaxRetries is the maximum number of retry attempts
	maxRetries?: int32 @protobuf(1,int32,name=max_retries)

	// BackoffStrategy determines how delays are calculated between retries
	backoffStrategy?: #BackoffStrategy @protobuf(2,BackoffStrategy,name=backoff_strategy)

	// InitialDelay is the delay before the first retry attempt
	initialDelay?: time.Duration @protobuf(3,google.protobuf.Duration,name=initial_delay)

	// MaxDelay is the maximum delay between retry attempts (used for exponential backoff)
	maxDelay?: time.Duration @protobuf(4,google.protobuf.Duration,name=max_delay)

	// Multiplier is the factor by which the delay increases (used for exponential backoff)
	multiplier?: float64 @protobuf(5,double)
}

// DataPolicy defines how data is handled for a node.
//
// Deprecated: superseded by the ECS brain (gibson#851, ADR-0008). Data
// handling (reuse + scoping) is now implicit in the event-sourced World and
// scope-relative identity (ADR-0002) + ambient projection. Retained
// wire-compatibly; ignored by the engine.
#DataPolicy: {
	@protobuf(option deprecated=true)

	// StoreInput determines whether to store input data in GraphRAG
	storeInput?: bool @protobuf(1,bool,name=store_input)

	// StoreOutput determines whether to store output data in GraphRAG
	storeOutput?: bool @protobuf(2,bool,name=store_output)

	// Retention specifies how long to retain data (0 = forever)
	retention?: time.Duration @protobuf(3,google.protobuf.Duration)

	// Encryption determines whether data should be encrypted at rest
	encryption?: bool @protobuf(4,bool)

	// AccessControl specifies who can access this data
	accessControl?: [...string] @protobuf(5,string,name=access_control)
}

// MissionEdge represents a directed edge in the mission DAG.
//
// The graph is acyclic. A loop is never an edge. Work that repeats until it
// passes an acceptance step loops INSIDE one node: a job node runs its
// verify loop against the same job (gibson#1706), and a node's RetryPolicy
// retries the node itself.
#MissionEdge: {
	// From is the source node ID
	from?: string @protobuf(1,string)

	// To is the destination node ID
	to?: string @protobuf(2,string)

	// Condition is an optional condition that must be satisfied for the edge to be traversed
	condition?: string @protobuf(3,string)

	// Metadata contains additional metadata for the edge
	metadata?: {
		[string]: string
	} @protobuf(4,map[string]string)
}
