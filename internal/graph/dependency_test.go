package graph

import (
	"reflect"
	"testing"
)

func hasDep(deps []DependencyEdge, blocked, blocker IssueID) bool {
	for _, d := range deps {
		if d.Blocked == blocked && d.Blocker == blocker {
			return true
		}
	}
	return false
}

func TestStalledEpicSurfacesBlockedTaskAndDependencyEdge(t *testing.T) {
	// epic 1 -> task 10, blocked by external ready blocker 99.
	g := newBuild().epic(1).open(10).open(99).child(1, 10).blocked(10, 99).g

	res := Compute(g, Options{})
	view, ok := viewFor(res, 1)
	if !ok || view.Status != StatusStalled {
		t.Fatalf("epic 1 view = %+v (ok=%v), want STALLED", view, ok)
	}
	if got := ids(view.Blockers); !reflect.DeepEqual(got, []IssueID{99}) {
		t.Fatalf("blockers = %v, want [99]", got)
	}
	if got := ids(view.Blocked); !reflect.DeepEqual(got, []IssueID{10}) {
		t.Fatalf("blocked = %v, want [10] (the held-up ladder task)", got)
	}
	// The real chain edge: #10 blocked by #99, both rendered.
	if !hasDep(res.Dependencies, 10, 99) {
		t.Fatalf("dependencies = %+v, want an edge {blocked:10, blocker:99}", res.Dependencies)
	}
}

func TestActiveEpicHasNoDependencyEdges(t *testing.T) {
	// epic 1 -> ready task 10 (no open blocker).
	g := newBuild().epic(1).open(10).child(1, 10).g
	res := Compute(g, Options{})
	if len(res.Dependencies) != 0 {
		t.Fatalf("dependencies = %+v, want none for a ready epic", res.Dependencies)
	}
	view, _ := viewFor(res, 1)
	if len(view.Blocked) != 0 {
		t.Fatalf("blocked = %v, want none for a ready epic", ids(view.Blocked))
	}
}

func TestDependencyEdgesOnlyAmongRenderedIssues(t *testing.T) {
	// 10 is blocked by 99 (ready, surfaced) and also by 50, which is out of any
	// epic's scope and therefore not rendered — its edge must not appear.
	g := newBuild().epic(1).open(10).open(99).child(1, 10).blocked(10, 99).g
	g.AddIssue(Issue{ID: 50, State: Open})
	g.AddDependency(10, 50) // 10 also blocked by 50; but 50 blocks nothing in scope

	res := Compute(g, Options{})
	// 50 is a blocker of a ladder task, so it IS in scope's blocker closure and
	// rendered — assert the edge set is exactly the rendered blocked-by pairs.
	if !hasDep(res.Dependencies, 10, 99) {
		t.Fatalf("missing edge 10->99 in %+v", res.Dependencies)
	}
	// Every dependency endpoint must be a rendered issue (no dangling edges).
	rendered := map[IssueID]bool{}
	for _, v := range res.Epics {
		rendered[v.Epic] = true
		for _, n := range append(append(append([]Node{}, v.Ready...), v.Blockers...), v.Blocked...) {
			rendered[n.ID] = true
		}
	}
	for _, d := range res.Dependencies {
		if !rendered[d.Blocked] || !rendered[d.Blocker] {
			t.Fatalf("dependency %+v references a non-rendered issue", d)
		}
	}
}
