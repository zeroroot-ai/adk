// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"context"
	"strings"
	"testing"

	daemonv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/daemon/v1"
	targetv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/target/v1"
	"google.golang.org/grpc"
)

// pagedTargets serves ListTargets from fixed pages, keyed by page token.
type pagedTargets struct {
	daemonv1.DaemonServiceClient
	pages map[string]*daemonv1.ListTargetsResponse
	asked []string
}

func (p *pagedTargets) ListTargets(
	_ context.Context, req *daemonv1.ListTargetsRequest, _ ...grpc.CallOption,
) (*daemonv1.ListTargetsResponse, error) {
	p.asked = append(p.asked, req.GetPageToken())
	return p.pages[req.GetPageToken()], nil
}

func pageTarget(id, name string) *targetv1.Target { return &targetv1.Target{Id: id, Name: name} }

// A target on the second page is found: the lookup reads each page.
func TestAllTargets_ReadsEachPage(t *testing.T) {
	c := &pagedTargets{pages: map[string]*daemonv1.ListTargetsResponse{
		"":   {Targets: []*targetv1.Target{pageTarget("a", "one")}, NextPageToken: "p2"},
		"p2": {Targets: []*targetv1.Target{pageTarget("b", "two")}},
	}}
	got, err := allTargets(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].GetName() != "two" {
		t.Fatalf("targets = %v, want both pages", got)
	}
	if strings.Join(c.asked, ",") != ",p2" {
		t.Errorf("page tokens asked = %q, want \"\" then p2", c.asked)
	}
}

// A server that returns the same token twice stops the loop with an error.
func TestAllTargets_StopsOnARepeatedToken(t *testing.T) {
	c := &pagedTargets{pages: map[string]*daemonv1.ListTargetsResponse{
		"":   {Targets: []*targetv1.Target{pageTarget("a", "one")}, NextPageToken: "p2"},
		"p2": {NextPageToken: "p2"},
	}}
	if _, err := allTargets(context.Background(), c); err == nil {
		t.Fatal("want an error for a repeated page token, got none")
	}
}
