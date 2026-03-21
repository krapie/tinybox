package scheduler_test

import (
	"testing"

	"github.com/krapi0314/tinybox/tinykube/scheduler"
)

func TestRoundRobinNoNodes(t *testing.T) {
	s := scheduler.NewRoundRobin()
	node := s.Select(nil)
	if node != "" {
		t.Fatalf("expected empty string when no nodes, got %s", node)
	}
}

func TestRoundRobinSingleNode(t *testing.T) {
	s := scheduler.NewRoundRobin()
	nodes := []string{"node1"}

	for i := 0; i < 5; i++ {
		node := s.Select(nodes)
		if node != "node1" {
			t.Fatalf("expected node1, got %s", node)
		}
	}
}

func TestRoundRobinMultipleNodes(t *testing.T) {
	s := scheduler.NewRoundRobin()
	nodes := []string{"node1", "node2", "node3"}

	results := make([]string, 6)
	for i := range results {
		results[i] = s.Select(nodes)
	}

	// Should cycle through all nodes.
	seen := map[string]int{}
	for _, r := range results {
		seen[r]++
	}
	for _, n := range nodes {
		if seen[n] == 0 {
			t.Fatalf("node %s was never selected", n)
		}
	}

	// First three should be distinct (round-robin).
	set := map[string]struct{}{results[0]: {}, results[1]: {}, results[2]: {}}
	if len(set) != 3 {
		t.Fatalf("expected all 3 nodes in first 3 selections, got %v", results[:3])
	}
}

func TestRoundRobinWrapsAround(t *testing.T) {
	s := scheduler.NewRoundRobin()
	nodes := []string{"node1", "node2"}

	first := s.Select(nodes)
	second := s.Select(nodes)
	third := s.Select(nodes) // should wrap back to first

	if first != third {
		t.Fatalf("expected round-robin to wrap: first=%s, third=%s", first, third)
	}
	if first == second {
		t.Fatalf("expected first and second to differ: got %s both times", first)
	}
}

func TestRoundRobinIndependentInstances(t *testing.T) {
	s1 := scheduler.NewRoundRobin()
	s2 := scheduler.NewRoundRobin()
	nodes := []string{"node1", "node2"}

	s1.Select(nodes)
	s1.Select(nodes)

	// s2 starts fresh
	first := s2.Select(nodes)
	if first != nodes[0] {
		t.Fatalf("expected s2 to start from beginning, got %s", first)
	}
}
