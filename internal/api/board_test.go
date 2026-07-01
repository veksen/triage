package api

import (
	"testing"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
	"github.com/veksen/triage/internal/graph"
)

const repoURL = "https://github.com/veksen/triage"

func TestBoardMapsActiveEpicWithEnrichedNode(t *testing.T) {
	g := graph.NewGraph()
	g.AddIssue(graph.Issue{ID: 1, Title: "Ship dashboard", State: graph.Open, IsEpic: true})
	g.AddIssue(graph.Issue{ID: 10, Title: "Wire the API", State: graph.Open})
	g.AddHierarchy(1, 10)

	board := NewBoardBuilder(g, repoURL).Board(graph.Compute(g, graph.Options{}))

	if len(board.GetEpics()) != 1 {
		t.Fatalf("epics = %d, want 1", len(board.GetEpics()))
	}
	ev := board.GetEpics()[0]
	if ev.GetEpic().GetNumber() != 1 || ev.GetEpic().GetTitle() != "Ship dashboard" {
		t.Fatalf("epic ref = %+v, want number 1 / title 'Ship dashboard'", ev.GetEpic())
	}
	if ev.GetStatus() != triagev1.EpicStatus_EPIC_STATUS_ACTIVE {
		t.Fatalf("status = %v, want ACTIVE", ev.GetStatus())
	}
	if len(ev.GetReady()) != 1 || len(ev.GetBlockers()) != 0 {
		t.Fatalf("ready=%d blockers=%d, want 1/0", len(ev.GetReady()), len(ev.GetBlockers()))
	}
	leaf := ev.GetReady()[0].GetIssue()
	if leaf.GetNumber() != 10 || leaf.GetTitle() != "Wire the API" || !leaf.GetOpen() {
		t.Fatalf("leaf ref = %+v, want number 10 / title 'Wire the API' / open", leaf)
	}
	if got, want := leaf.GetUrl(), repoURL+"/issues/10"; got != want {
		t.Fatalf("leaf url = %q, want %q", got, want)
	}
}

func TestBoardMapsStallToBlockers(t *testing.T) {
	g := graph.NewGraph()
	g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
	g.AddIssue(graph.Issue{ID: 10, Title: "Blocked task", State: graph.Open})
	g.AddIssue(graph.Issue{ID: 99, Title: "External prerequisite", State: graph.Open})
	g.AddHierarchy(1, 10)
	g.AddDependency(10, 99) // 10 blocked by 99 (out of scope)

	board := NewBoardBuilder(g, "").Board(graph.Compute(g, graph.Options{}))
	ev := board.GetEpics()[0]

	if ev.GetStatus() != triagev1.EpicStatus_EPIC_STATUS_STALLED {
		t.Fatalf("status = %v, want STALLED", ev.GetStatus())
	}
	if len(ev.GetReady()) != 0 {
		t.Fatalf("stalled epic should have no ready leaves, got %d", len(ev.GetReady()))
	}
	if len(ev.GetBlockers()) != 1 || ev.GetBlockers()[0].GetIssue().GetNumber() != 99 {
		t.Fatalf("blockers = %+v, want single blocker #99", ev.GetBlockers())
	}
	// No repoURL configured -> URL omitted.
	if url := ev.GetBlockers()[0].GetIssue().GetUrl(); url != "" {
		t.Fatalf("url = %q, want empty when repoURL unset", url)
	}
}

func TestBoardMapsBlockedTasksAndDependencyEdges(t *testing.T) {
	g := graph.NewGraph()
	g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
	g.AddIssue(graph.Issue{ID: 10, Title: "Blocked task", State: graph.Open})
	g.AddIssue(graph.Issue{ID: 99, Title: "External prerequisite", State: graph.Open})
	g.AddHierarchy(1, 10)
	g.AddDependency(10, 99) // 10 blocked by 99

	board := NewBoardBuilder(g, repoURL).Board(graph.Compute(g, graph.Options{}))
	ev := board.GetEpics()[0]

	if len(ev.GetBlocked()) != 1 || ev.GetBlocked()[0].GetIssue().GetNumber() != 10 {
		t.Fatalf("blocked = %+v, want single held-up task #10", ev.GetBlocked())
	}
	deps := board.GetDependencies()
	if len(deps) != 1 || deps[0].GetBlocked() != 10 || deps[0].GetBlocker() != 99 {
		t.Fatalf("dependencies = %+v, want single edge {blocked:10, blocker:99}", deps)
	}
}

func TestBoardEmptyWhenNoActiveEpics(t *testing.T) {
	g := graph.NewGraph()
	g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true, Labels: []string{"parked"}})
	g.AddIssue(graph.Issue{ID: 10, State: graph.Open})
	g.AddHierarchy(1, 10)

	board := NewBoardBuilder(g, repoURL).Board(graph.Compute(g, graph.Options{}))
	if len(board.GetEpics()) != 0 {
		t.Fatalf("epics = %d, want 0 (only epic is parked)", len(board.GetEpics()))
	}
}
