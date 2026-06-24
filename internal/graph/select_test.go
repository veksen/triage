package graph

import (
	"reflect"
	"sort"
	"testing"
)

// --- fixture helpers -------------------------------------------------------

type build struct{ g *Graph }

func newBuild() *build { return &build{g: NewGraph()} }

func (b *build) epic(id IssueID, labels ...string) *build {
	b.g.AddIssue(Issue{ID: id, State: Open, IsEpic: true, Labels: labels})
	return b
}

func (b *build) open(id IssueID, labels ...string) *build {
	b.g.AddIssue(Issue{ID: id, State: Open, Labels: labels})
	return b
}

func (b *build) closed(id IssueID) *build {
	b.g.AddIssue(Issue{ID: id, State: Closed})
	return b
}

// child wires parent -> child hierarchy.
func (b *build) child(parent, child IssueID) *build {
	b.g.AddHierarchy(parent, child)
	return b
}

// blocked wires "a is blocked by b".
func (b *build) blocked(a, by IssueID) *build {
	b.g.AddDependency(a, by)
	return b
}

// epicView returns the view for a given epic id.
func viewFor(r Result, epic IssueID) (EpicView, bool) {
	for _, v := range r.Epics {
		if v.Epic == epic {
			return v, true
		}
	}
	return EpicView{}, false
}

func ids(nodes []Node) []IssueID {
	out := make([]IssueID, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func nodeByID(nodes []Node, id IssueID) (Node, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// --- tests -----------------------------------------------------------------

func TestEmptyActiveSetYieldsEmptyResult(t *testing.T) {
	// A parked epic and a closed epic — neither drives the board.
	g := newBuild().
		epic(1, "parked").
		epic(2).
		child(1, 10).open(10).
		build()
	g.g.AddIssue(Issue{ID: 2, State: Closed, IsEpic: true}) // closed epic
	g.g.AddHierarchy(2, 20)
	g.g.AddIssue(Issue{ID: 20, State: Open})

	res := Compute(g.g, Options{})
	if len(res.Epics) != 0 {
		t.Fatalf("expected no epic views, got %d: %+v", len(res.Epics), res.Epics)
	}
}

func TestReadyFrontierForUnblockedLadder(t *testing.T) {
	// Epic 1 with two open, unblocked tasks -> both are ready.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		child(1, 11).open(11).
		build()

	v, ok := viewFor(Compute(g.g, Options{}), 1)
	if !ok || v.Status != StatusActive {
		t.Fatalf("expected active epic, got ok=%v status=%v", ok, v.Status)
	}
	if got, want := ids(v.Ready), []IssueID{10, 11}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ready frontier = %v, want %v", got, want)
	}
}

func TestBlockerInsideLadderIsTheFrontier(t *testing.T) {
	// Task 10 is blocked by sibling 11; only 11 is ready. Epic still active.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		child(1, 11).open(11).
		blocked(10, 11).
		build()

	v, _ := viewFor(Compute(g.g, Options{}), 1)
	if v.Status != StatusActive {
		t.Fatalf("status = %v, want active", v.Status)
	}
	if got, want := ids(v.Ready), []IssueID{11}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ready = %v, want %v (only the unblocked sibling)", got, want)
	}
}

func TestStallSurfacesCrossEpicBlocker(t *testing.T) {
	// Epic 1's only task is blocked by issue 99, which lives outside the epic's
	// ladder and is itself ready. The epic stalls and surfaces 99.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		open(99).
		blocked(10, 99).
		build()

	v, _ := viewFor(Compute(g.g, Options{}), 1)
	if v.Status != StatusStalled {
		t.Fatalf("status = %v, want stalled", v.Status)
	}
	if len(v.Ready) != 0 {
		t.Fatalf("stalled epic should have no ready leaves, got %v", ids(v.Ready))
	}
	if got, want := ids(v.Blockers), []IssueID{99}; !reflect.DeepEqual(got, want) {
		t.Fatalf("blockers = %v, want %v", got, want)
	}
}

func TestStallSurfacesNearestActionableBlockerInChain(t *testing.T) {
	// 10 <- 98 <- 99 (10 blocked by 98, 98 blocked by 99). Only 99 is actionable;
	// 98 is itself blocked. The stall surfaces 99, not 98.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		open(98).open(99).
		blocked(10, 98).
		blocked(98, 99).
		build()

	v, _ := viewFor(Compute(g.g, Options{}), 1)
	if v.Status != StatusStalled {
		t.Fatalf("status = %v, want stalled", v.Status)
	}
	if got, want := ids(v.Blockers), []IssueID{99}; !reflect.DeepEqual(got, want) {
		t.Fatalf("blockers = %v, want %v (nearest actionable, not the blocked middle)", got, want)
	}
}

func TestClosedIssuesAreHidden(t *testing.T) {
	// A closed task and a closed-out branch never appear.
	g := newBuild().
		epic(1).
		child(1, 10).closed(10).
		child(1, 11).open(11).
		build()

	v, _ := viewFor(Compute(g.g, Options{}), 1)
	if got, want := ids(v.Ready), []IssueID{11}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ready = %v, want %v (closed 10 hidden)", got, want)
	}
}

func TestAllChildrenClosedIsEmptyEpic(t *testing.T) {
	g := newBuild().
		epic(1).
		child(1, 10).closed(10).
		build()

	v, _ := viewFor(Compute(g.g, Options{}), 1)
	if v.Status != StatusEmpty {
		t.Fatalf("status = %v, want empty", v.Status)
	}
}

func TestLeverageRanksHighUnlockFirst(t *testing.T) {
	// Both 10 and 11 are ready. 10 unblocks 20, 21, 22 (in-scope via ladder);
	// 11 unblocks nothing. 10 should rank first and be flagged high-leverage.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		child(1, 11).open(11).
		child(1, 20).open(20).
		child(1, 21).open(21).
		child(1, 22).open(22).
		blocked(20, 10).
		blocked(21, 10).
		blocked(22, 10).
		build()

	v, _ := viewFor(Compute(g.g, Options{}), 1)
	if v.Ready[0].ID != 10 {
		t.Fatalf("top ready = %d, want 10 (highest leverage)", v.Ready[0].ID)
	}
	n10, _ := nodeByID(v.Ready, 10)
	if n10.Leverage != 3 {
		t.Fatalf("leverage(10) = %d, want 3", n10.Leverage)
	}
	if !n10.HighLeverage {
		t.Fatalf("node 10 should be flagged high-leverage at threshold 3")
	}
	n11, _ := nodeByID(v.Ready, 11)
	if n11.HighLeverage {
		t.Fatalf("node 11 (leverage 0) should not be high-leverage")
	}
}

func TestTopNCapsReadyLeaves(t *testing.T) {
	b := newBuild().epic(1)
	for id := IssueID(10); id < 16; id++ {
		b = b.child(1, id).open(id)
	}
	g := b.build()

	v, _ := viewFor(Compute(g.g, Options{TopN: 2}), 1)
	if len(v.Ready) != 2 {
		t.Fatalf("ready len = %d, want 2 (TopN cap)", len(v.Ready))
	}
}

func TestMultiEpicNodeIsFlaggedAndServesBoth(t *testing.T) {
	// Shared task 50 is a descendant of epic 1 and a blocker of epic 2's task.
	// It must surface for both and be flagged multi-epic.
	g := newBuild().
		epic(1).
		epic(2).
		child(1, 50).open(50).
		child(2, 60).open(60).
		blocked(60, 50). // epic 2's task 60 is blocked by shared task 50
		build()

	res := Compute(g.g, Options{})

	v1, _ := viewFor(res, 1)
	n, ok := nodeByID(v1.Ready, 50)
	if !ok {
		t.Fatalf("node 50 should be in epic 1's ready frontier")
	}
	if !n.MultiEpic {
		t.Fatalf("node 50 should be flagged multi-epic")
	}
	if got, want := append([]IssueID(nil), n.ServesEpics...), []IssueID{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("serves = %v, want %v", got, want)
	}

	// Epic 2 is stalled on 50 (its own task 60 is blocked) and surfaces it.
	v2, _ := viewFor(res, 2)
	if v2.Status != StatusStalled {
		t.Fatalf("epic 2 status = %v, want stalled", v2.Status)
	}
	if got, want := ids(v2.Blockers), []IssueID{50}; !reflect.DeepEqual(got, want) {
		t.Fatalf("epic 2 blockers = %v, want %v", got, want)
	}
}

func TestAncestryDepthAttachesContext(t *testing.T) {
	// epic 1 -> story 10 -> task 100. Task is ready; ancestry walks up.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		child(10, 100).open(100).
		build()

	depth2, _ := viewFor(Compute(g.g, Options{AncestryDepth: 2}), 1)
	n, _ := nodeByID(depth2.Ready, 100)
	if got, want := n.Ancestry, []IssueID{10, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ancestry depth 2 = %v, want %v", got, want)
	}

	depth1, _ := viewFor(Compute(g.g, Options{AncestryDepth: 1}), 1)
	n1, _ := nodeByID(depth1.Ready, 100)
	if got, want := n1.Ancestry, []IssueID{10}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ancestry depth 1 = %v, want %v", got, want)
	}
}

func TestParkedEpicIsExcluded(t *testing.T) {
	g := newBuild().
		epic(1, "parked").
		epic(2).
		child(1, 10).open(10).
		child(2, 20).open(20).
		build()

	res := Compute(g.g, Options{})
	if _, ok := viewFor(res, 1); ok {
		t.Fatalf("parked epic 1 should not render")
	}
	if _, ok := viewFor(res, 2); !ok {
		t.Fatalf("active epic 2 should render")
	}
}

func TestDependencyCycleTerminates(t *testing.T) {
	// Dirty data: 10 and 11 block each other. Must not loop forever; both are
	// blocked, so the epic stalls with no actionable blocker.
	g := newBuild().
		epic(1).
		child(1, 10).open(10).
		child(1, 11).open(11).
		blocked(10, 11).
		blocked(11, 10).
		build()

	done := make(chan EpicView, 1)
	go func() {
		v, _ := viewFor(Compute(g.g, Options{}), 1)
		done <- v
	}()
	v := <-done // a non-terminating traversal would hang here
	if v.Status != StatusStalled {
		t.Fatalf("status = %v, want stalled (mutual block)", v.Status)
	}
	if len(v.Ready) != 0 || len(v.Blockers) != 0 {
		t.Fatalf("cycle should leave nothing actionable, got ready=%v blockers=%v", ids(v.Ready), ids(v.Blockers))
	}
}

// build finalizes the fixture.
func (b *build) build() *build { return b }
