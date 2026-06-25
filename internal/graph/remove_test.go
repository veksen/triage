package graph

import "testing"

func TestRemoveHierarchyUnlinksParentAndChild(t *testing.T) {
	g := NewGraph()
	g.AddIssue(Issue{ID: 1, State: Open, IsEpic: true})
	g.AddIssue(Issue{ID: 10, State: Open})
	g.AddHierarchy(1, 10)

	g.RemoveHierarchy(1, 10)

	// The child is no longer in the epic's ladder, so the epic is EMPTY.
	res := Compute(g, Options{})
	if len(res.Epics) != 1 || res.Epics[0].Status != StatusEmpty {
		t.Fatalf("after RemoveHierarchy want one EMPTY epic, got %+v", res.Epics)
	}
	if _, ok := g.parent[10]; ok {
		t.Fatal("child still has a parent pointer after RemoveHierarchy")
	}
}

func TestRemoveHierarchyKeepsReparentTarget(t *testing.T) {
	g := NewGraph()
	g.AddHierarchy(1, 10)
	g.AddHierarchy(2, 10) // re-parent: 10 now points at 2
	g.RemoveHierarchy(1, 10)

	if got := g.parent[10]; got != 2 {
		t.Fatalf("parent[10] = %d after removing the stale link, want 2", got)
	}
}

func TestRemoveDependencyUnblocks(t *testing.T) {
	g := NewGraph()
	g.AddIssue(Issue{ID: 1, State: Open, IsEpic: true})
	g.AddIssue(Issue{ID: 10, State: Open})
	g.AddIssue(Issue{ID: 99, State: Open})
	g.AddHierarchy(1, 10)
	g.AddDependency(10, 99) // 10 blocked by 99 -> epic stalled

	if Compute(g, Options{}).Epics[0].Status != StatusStalled {
		t.Fatal("precondition: epic should be STALLED while 10 is blocked")
	}

	g.RemoveDependency(10, 99)

	if got := Compute(g, Options{}).Epics[0].Status; got != StatusActive {
		t.Fatalf("status = %v after RemoveDependency, want ACTIVE", got)
	}
}

func TestRemoveIssueDetachesAllEdges(t *testing.T) {
	g := NewGraph()
	g.AddIssue(Issue{ID: 1, State: Open, IsEpic: true})
	g.AddIssue(Issue{ID: 10, State: Open})
	g.AddIssue(Issue{ID: 99, State: Open})
	g.AddHierarchy(1, 10)
	g.AddDependency(10, 99)

	g.RemoveIssue(10)

	if _, ok := g.Issue(10); ok {
		t.Fatal("issue 10 still present after RemoveIssue")
	}
	// 10's edges are gone in both directions.
	if len(g.children[1]) != 0 {
		t.Fatalf("epic 1 still lists children %v after its only child was removed", g.children[1])
	}
	if len(g.blocks[99]) != 0 {
		t.Fatalf("99 still records blocking %v after the blocked issue was removed", g.blocks[99])
	}
	// Epic now has no ladder work.
	if got := Compute(g, Options{}).Epics[0].Status; got != StatusEmpty {
		t.Fatalf("status = %v after removing the only ladder issue, want EMPTY", got)
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	g := NewGraph()
	g.AddIssue(Issue{ID: 1, State: Open})
	g.AddIssue(Issue{ID: 2, State: Open})
	g.AddHierarchy(1, 2)
	g.AddDependency(2, 1)

	// Removing absent edges / issues must not panic and must stay a no-op.
	g.RemoveHierarchy(1, 2)
	g.RemoveHierarchy(1, 2)
	g.RemoveDependency(2, 1)
	g.RemoveDependency(2, 1)
	g.RemoveIssue(1)
	g.RemoveIssue(1)
	g.RemoveIssue(404)
}
