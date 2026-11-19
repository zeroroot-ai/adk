// Command demo-agent is a Gibson agent.
//
// See AGENTS.md for the full Gibson agent contract this binary implements.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/a2aproject/a2a-go/v2/a2a"
	sdk "github.com/zeroroot-ai/sdk"
	"github.com/zeroroot-ai/sdk/agent"
	"github.com/zeroroot-ai/sdk/openresponses"
)

func main() {
	a, err := sdk.NewAgent(
		sdk.WithName("demo-agent"),
		sdk.WithVersion("0.1.0"),
		sdk.WithDescription("demo-agent agent"),
		sdk.WithExecuteFunc(execute),
	)
	if err != nil {
		slog.Error("create agent", "err", err)
		os.Exit(1)
	}

	// ServeAgent polls the daemon for work. The daemon sends each task as an
	// A2A message when this agent's node in a mission DAG becomes active.
	// The harness arg in execute() is provisioned by the daemon. It gives
	// the agent access to the model, tools, plugins, and the three-tier
	// memory.
	if err := sdk.ServeAgent(a); err != nil {
		slog.Error("serve agent", "err", err)
		os.Exit(1)
	}
}

// execute is the agent's main reasoning loop. It makes one real Harness
// call: it gets the model client of the primary slot and asks it about the
// goal of the task. The agent holds its own item list. Replace the
// prompt with your own logic. See AGENTS.md for harness API patterns and
// core/sdk/agent/harness.go for the full surface.
func execute(ctx context.Context, h agent.Harness, task agent.Task) (agent.Result, error) {
	slog.InfoContext(ctx, "executing task", "id", task.ID, "goal", task.Goal)

	model, err := h.Model(ctx, "primary")
	if err != nil {
		return agent.Result{Status: a2a.TaskStateFailed}, fmt.Errorf("resolve model: %w", err)
	}
	items := []openresponses.Item{
		openresponses.NewSystemMessage("You are demo-agent."),
		openresponses.NewUserMessage(task.Goal),
	}
	resp, err := model.Create(ctx, &openresponses.Request{Input: items})
	if err != nil {
		return agent.Result{Status: a2a.TaskStateFailed}, fmt.Errorf("complete: %w", err)
	}

	return agent.Result{
		Status: a2a.TaskStateCompleted,
		Output: resp.Text(),
	}, nil
}
