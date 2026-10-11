package main

import (
	"context"
	"errors"
	"testing"

	"github.com/zeroroot-ai/sdk/agent"
	"github.com/zeroroot-ai/sdk/llm"
)

// stubHarness answers Complete and nothing else. The embedded interface is
// nil, so a call to any other Harness method panics and the test shows it.
type stubHarness struct {
	agent.Harness
	reply string
	err   error
	got   []llm.Message
}

func (s *stubHarness) Complete(
	_ context.Context, slot string, messages []llm.Message, _ ...llm.CompletionOption,
) (*llm.CompletionResponse, error) {
	if slot != "primary" {
		return nil, errors.New("unexpected slot " + slot)
	}
	s.got = messages
	if s.err != nil {
		return nil, s.err
	}
	return &llm.CompletionResponse{Content: s.reply}, nil
}

func TestExecuteAsksTheLLMAboutTheGoal(t *testing.T) {
	h := &stubHarness{reply: "done"}
	res, err := execute(context.Background(), h, agent.Task{ID: "t1", Goal: "scan the host"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Status != agent.StatusSuccess || res.Output != "done" {
		t.Fatalf("result = %+v", res)
	}
	if len(h.got) != 2 || h.got[1].Content != "scan the host" {
		t.Fatalf("messages = %+v", h.got)
	}
}

func TestExecuteReportsAnLLMFailure(t *testing.T) {
	h := &stubHarness{err: errors.New("no model")}
	res, err := execute(context.Background(), h, agent.Task{ID: "t1", Goal: "x"})
	if err == nil || res.Status != agent.StatusFailed {
		t.Fatalf("got %+v, %v", res, err)
	}
}
