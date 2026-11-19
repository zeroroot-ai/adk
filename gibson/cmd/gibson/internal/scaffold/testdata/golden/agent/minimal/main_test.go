package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/zeroroot-ai/sdk/agent"
	"github.com/zeroroot-ai/sdk/openresponses"
)

// stubHarness answers Model and nothing else. The embedded interface is nil,
// so a call to any other Harness method panics and the test shows it. Model
// returns a client of a test gateway that sends back one fixed reply.
type stubHarness struct {
	agent.Harness
	t     *testing.T
	reply string
	err   error
	input []wireItem
}

// wireItem is one input item that the test gateway received.
type wireItem struct {
	Role    string `json:"role"`
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (s *stubHarness) Model(_ context.Context, slot string) (*openresponses.Client, error) {
	if slot != "primary" {
		return nil, errors.New("unexpected slot " + slot)
	}
	if s.err != nil {
		return nil, s.err
	}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []wireItem `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.t.Errorf("decode request: %v", err)
		}
		s.input = req.Input
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_1",
			"status": "completed",
			"output": []any{map[string]any{
				"type": "message", "id": "msg_1", "role": "assistant", "status": "completed",
				"content": []any{map[string]any{"type": "output_text", "text": s.reply}},
			}},
		})
	}))
	s.t.Cleanup(gateway.Close)
	return &openresponses.Client{BaseURL: gateway.URL, Model: "test-model"}, nil
}

func TestExecuteAsksTheModelAboutTheGoal(t *testing.T) {
	h := &stubHarness{t: t, reply: "done"}
	res, err := execute(context.Background(), h, agent.Task{ID: "t1", Goal: "scan the host"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Status != a2a.TaskStateCompleted || res.Output != "done" {
		t.Fatalf("result = %+v", res)
	}
	if len(h.input) != 2 || h.input[0].Role != "system" || h.input[1].Content[0].Text != "scan the host" {
		t.Fatalf("input items = %+v", h.input)
	}
}

func TestExecuteReportsAModelFailure(t *testing.T) {
	h := &stubHarness{t: t, err: errors.New("no model")}
	res, err := execute(context.Background(), h, agent.Task{ID: "t1", Goal: "x"})
	if err == nil || res.Status != a2a.TaskStateFailed {
		t.Fatalf("got %+v, %v", res, err)
	}
}
