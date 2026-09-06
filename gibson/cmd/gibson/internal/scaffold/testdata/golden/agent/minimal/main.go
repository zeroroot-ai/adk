// Command demo-agent is a Gibson agent.
//
// See AGENTS.md for the full Gibson agent contract this binary implements.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	sdk "github.com/zeroroot-ai/sdk"
	"github.com/zeroroot-ai/sdk/agent"
	"github.com/zeroroot-ai/sdk/llm"
)

func main() {
	a, err := sdk.NewAgent(
		sdk.WithName("demo-agent"),
		sdk.WithVersion("0.1.0"),
		sdk.WithDescription("demo-agent agent"),
		// Declare the LLM slot the agent expects the harness to fill.
		// See core/sdk/llm/ for the full slot-requirement shape.
		sdk.WithLLMSlot("primary", llm.SlotRequirements{
			MinContextWindow: 8000,
		}),
		sdk.WithExecuteFunc(execute),
	)
	if err != nil {
		slog.Error("create agent", "err", err)
		os.Exit(1)
	}

	// ServeAgent starts a gRPC server that the daemon dials when this
	// agent's node in a mission DAG becomes active. The harness arg in
	// execute() is provisioned by the daemon — it gives the agent
	// access to LLMs, tools, plugins, and the three-tier memory.
	if err := sdk.ServeAgent(a); err != nil {
		slog.Error("serve agent", "err", err)
		os.Exit(1)
	}
}

// execute is the agent's main reasoning loop. Replace the stub with
// your real logic. See AGENTS.md for harness API patterns and
// core/sdk/agent/harness.go for the full surface.
func execute(ctx context.Context, h agent.Harness, task agent.Task) (agent.Result, error) {
	slog.InfoContext(ctx, "executing task", "id", task.ID, "goal", task.Goal)

	// Example: call the primary LLM with the task's goal.
	// Uncomment and adapt — this stub returns a static result so the
	// scaffold compiles before the developer wires in real behaviour.
	//
	// resp, err := h.Complete(ctx, "primary", []llm.Message{
	// 	{Role: llm.RoleSystem, Content: "You are demo-agent."},
	// 	{Role: llm.RoleUser,   Content: task.Goal},
	// })
	// if err != nil {
	// 	return agent.Result{Status: agent.StatusFailed}, err
	// }

	return agent.Result{
		Status: agent.StatusSuccess,
		Output: fmt.Sprintf("agent demo-agent executed task %s", task.ID),
	}, nil
}
