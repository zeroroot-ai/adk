// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agent

import (
	"context"
	"testing"

	agentidentityv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/agentidentity/v1"
	"google.golang.org/grpc"
)

// pagedIdentities serves ListAgentIdentities from fixed pages.
type pagedIdentities struct {
	agentidentityv1.AgentIdentityServiceClient
	pages   map[string]*agentidentityv1.ListAgentIdentitiesResponse
	filters []agentidentityv1.PrincipalKind
}

func (p *pagedIdentities) ListAgentIdentities(
	_ context.Context, req *agentidentityv1.ListAgentIdentitiesRequest, _ ...grpc.CallOption,
) (*agentidentityv1.ListAgentIdentitiesResponse, error) {
	p.filters = append(p.filters, req.GetKindFilter())
	return p.pages[req.GetPageToken()], nil
}

// Each page is read, and the kind filter stays on each request.
func TestAllIdentities_ReadsEachPageWithTheFilter(t *testing.T) {
	c := &pagedIdentities{pages: map[string]*agentidentityv1.ListAgentIdentitiesResponse{
		"":   {Identities: []*agentidentityv1.AgentIdentity{{Name: "a"}}, NextPageToken: "p2"},
		"p2": {Identities: []*agentidentityv1.AgentIdentity{{Name: "b"}}},
	}}
	req := &agentidentityv1.ListAgentIdentitiesRequest{KindFilter: agentidentityv1.PrincipalKind(1)}
	got, err := allIdentities(context.Background(), c, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("identities = %d, want 2", len(got))
	}
	for i, f := range c.filters {
		if f != agentidentityv1.PrincipalKind(1) {
			t.Errorf("request %d lost the kind filter: %v", i, f)
		}
	}
}

// A server that returns the same token twice stops the loop with an error.
func TestAllIdentities_StopsOnARepeatedToken(t *testing.T) {
	c := &pagedIdentities{pages: map[string]*agentidentityv1.ListAgentIdentitiesResponse{
		"":   {NextPageToken: "p2"},
		"p2": {NextPageToken: "p2"},
	}}
	if _, err := allIdentities(context.Background(), c, &agentidentityv1.ListAgentIdentitiesRequest{}); err == nil {
		t.Fatal("want an error for a repeated page token, got none")
	}
}
