// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package mission

import (
	"fmt"
	"sort"

	missionv1 "github.com/zeroroot-ai/sdk/api/gen/gibson/mission/v1"
)

// checkStartsFrom applies the graph rule of starts_from (ADR-0169). A node
// that names a node in starts_from begins from the state that the named node
// had when it ended. So the named node must exist, must not be the node
// itself, and must end before this node starts: it must be an ancestor of
// the node through `dependencies` or `edges`.
//
// The template node of a for_each node can carry starts_from. Its instances
// start when the for_each node starts, so the rule applies to the for_each
// node: the named node must be an ancestor of the for_each node.
//
// protovalidate cannot express a rule about a different node of the graph,
// so the daemon applies the same rule at submit time. This check gives the
// author the answer before the submit.
func checkStartsFrom(def *missionv1.MissionDefinition) error {
	nodes := def.GetNodes()
	parents := make(map[string][]string, len(nodes))
	for id, n := range nodes {
		parents[id] = append(parents[id], n.GetDependencies()...)
	}
	for _, e := range def.GetEdges() {
		parents[e.GetTo()] = append(parents[e.GetTo()], e.GetFrom())
	}

	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		n := nodes[id]
		if err := checkOneStartsFrom(id, id, n.GetStartsFrom(), nodes, parents); err != nil {
			return err
		}
		if tpl := n.GetForEachConfig().GetTemplate(); tpl != nil {
			label := fmt.Sprintf("the template of %q", id)
			if err := checkOneStartsFrom(label, id, tpl.GetStartsFrom(), nodes, parents); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkOneStartsFrom checks one starts_from value. label names the node in
// the message. anchor is the node whose ancestors are searched.
func checkOneStartsFrom(label, anchor, from string, nodes map[string]*missionv1.MissionNode,
	parents map[string][]string,
) error {
	if from == "" {
		return nil
	}
	if _, ok := nodes[from]; !ok {
		return fmt.Errorf("node %s: starts_from %q names no node of the mission", label, from)
	}
	if from == anchor {
		return fmt.Errorf("node %s: starts_from names the node itself; name an earlier node", label)
	}
	if !isAncestor(from, anchor, parents) {
		return fmt.Errorf("node %s: starts_from %q is not an earlier node: add %q to its dependencies, "+
			"or to the dependencies of a node that it depends on", label, from, from)
	}
	return nil
}

// isAncestor reports whether want is reachable from node by following the
// parent links. A cycle ends the walk, because each node is visited once.
func isAncestor(want, node string, parents map[string][]string) bool {
	seen := map[string]bool{node: true}
	stack := append([]string(nil), parents[node]...)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if p == want {
			return true
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		stack = append(stack, parents[p]...)
	}
	return false
}
