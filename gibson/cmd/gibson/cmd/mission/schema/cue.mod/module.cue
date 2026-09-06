// CUE module declaration for the embedded schema bundle.
// Module name matches the upstream SDK so templates can write:
//
//   import mission "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"
//
// and have it resolve to the embedded definitions without requiring
// the SDK source tree or a network fetch at validation time.
//
// This file is embedded into the adk CLI binary via //go:embed.

module: "github.com/zeroroot-ai/sdk@v1"

language: {
	version: "v0.16.0"
}
