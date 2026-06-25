package main

import "github.com/veksen/triage/internal/graph"

// devSeed builds a small illustrative graph covering all three epic states
// (ACTIVE / STALLED / EMPTY) so the frontend has something to render without a
// real GitHub backfill. Enabled with TRIAGE_DEV_SEED; never used in production.
func devSeed() *graph.Graph {
	g := graph.NewGraph()
	epic := func(id int, title string) {
		g.AddIssue(graph.Issue{ID: graph.IssueID(id), Title: title, State: graph.Open, IsEpic: true, Labels: []string{"epic"}})
	}
	open := func(id int, title string) {
		g.AddIssue(graph.Issue{ID: graph.IssueID(id), Title: title, State: graph.Open})
	}
	closed := func(id int, title string) {
		g.AddIssue(graph.Issue{ID: graph.IssueID(id), Title: title, State: graph.Closed})
	}

	// Epic 1 — ACTIVE: two ready leaves, one carrying leverage.
	epic(1, "Ship dashboard")
	open(10, "Wire the API")
	open(11, "Design board UI")
	open(12, "Integrate frontend")
	g.AddHierarchy(1, 10)
	g.AddHierarchy(1, 11)
	g.AddHierarchy(1, 12)
	g.AddDependency(12, 10) // 12 blocked by 10 -> 10 gains leverage, 12 isn't ready

	// Epic 2 — STALLED: its only task is blocked by an out-of-scope prerequisite.
	epic(2, "Auth revamp")
	open(20, "Migrate sessions")
	open(99, "Vendor SSO rollout")
	g.AddHierarchy(2, 20)
	g.AddDependency(20, 99)

	// Epic 3 — EMPTY: all ladder work is done.
	epic(3, "Docs refresh")
	closed(30, "Rewrite README")
	g.AddHierarchy(3, 30)

	return g
}
