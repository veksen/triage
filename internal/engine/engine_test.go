package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
	"github.com/veksen/triage/internal/graph"
)

// fixture builds an engine over one open epic (#1) with one open, unblocked
// child (#10) — i.e. an ACTIVE epic with a single ready leaf.
func fixture() *Engine {
	g := graph.NewGraph()
	g.AddIssue(graph.Issue{ID: 1, Title: "Ship dashboard", State: graph.Open, IsEpic: true})
	g.AddIssue(graph.Issue{ID: 10, Title: "Wire the API", State: graph.Open})
	g.AddHierarchy(1, 10)
	return New(g, Config{RepoURL: "https://github.com/veksen/triage"})
}

func TestNewComputesInitialBoard(t *testing.T) {
	e := fixture()
	board := e.Board()
	if len(board.GetEpics()) != 1 {
		t.Fatalf("epics = %d, want 1", len(board.GetEpics()))
	}
	if got := board.GetEpics()[0].GetStatus(); got != triagev1.EpicStatus_EPIC_STATUS_ACTIVE {
		t.Fatalf("status = %v, want ACTIVE", got)
	}
}

func TestEmptyEngineServesEmptyBoard(t *testing.T) {
	e := New(nil, Config{})
	if n := len(e.Board().GetEpics()); n != 0 {
		t.Fatalf("epics = %d, want 0 for an empty graph", n)
	}
}

func TestApplyRecomputesBoard(t *testing.T) {
	e := New(nil, Config{})
	e.Apply(func(g *graph.Graph) {
		g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
		g.AddIssue(graph.Issue{ID: 10, State: graph.Open})
		g.AddHierarchy(1, 10)
	})
	if n := len(e.Board().GetEpics()); n != 1 {
		t.Fatalf("epics = %d, want 1 after Apply added an epic", n)
	}
}

func TestSetEpicStateParkRemovesEpicAndUnparkRestores(t *testing.T) {
	e := fixture()

	status, ok := e.SetEpicState(1, false) // park
	if !ok {
		t.Fatal("SetEpicState(park) ok = false, want true for known epic")
	}
	if status != triagev1.EpicStatus_EPIC_STATUS_UNSPECIFIED {
		t.Fatalf("parked status = %v, want UNSPECIFIED (off the board)", status)
	}
	if n := len(e.Board().GetEpics()); n != 0 {
		t.Fatalf("epics = %d after park, want 0", n)
	}

	status, ok = e.SetEpicState(1, true) // unpark
	if !ok {
		t.Fatal("SetEpicState(unpark) ok = false, want true")
	}
	if status != triagev1.EpicStatus_EPIC_STATUS_ACTIVE {
		t.Fatalf("unparked status = %v, want ACTIVE", status)
	}
	if n := len(e.Board().GetEpics()); n != 1 {
		t.Fatalf("epics = %d after unpark, want 1", n)
	}
}

func TestSetEpicStateUnknownEpic(t *testing.T) {
	e := New(nil, Config{})
	if _, ok := e.SetEpicState(404, true); ok {
		t.Fatal("SetEpicState on unknown epic ok = true, want false")
	}
}

func TestSetEpicStateIsIdempotentOnLabels(t *testing.T) {
	e := fixture()
	e.SetEpicState(1, false) // park
	e.SetEpicState(1, false) // park again — must not duplicate the label
	iss, _ := e.g.Issue(1)
	count := 0
	for _, l := range iss.Labels {
		if l == "parked" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("parked label appears %d times, want 1 (no duplication)", count)
	}
}

func TestSubscribePrimesWithCurrentBoard(t *testing.T) {
	e := fixture()
	sub, cancel := e.Subscribe()
	defer cancel()

	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	board, ok := sub.Next(ctx)
	if !ok {
		t.Fatal("Next timed out, want the primed snapshot")
	}
	if len(board.GetEpics()) != 1 {
		t.Fatalf("primed epics = %d, want 1", len(board.GetEpics()))
	}
}

func TestSubscribeReceivesUpdatesOnApply(t *testing.T) {
	e := New(nil, Config{})
	sub, cancel := e.Subscribe()
	defer cancel()

	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()

	// Drain the (empty) primed snapshot first so registration precedes Apply.
	if _, ok := sub.Next(ctx); !ok {
		t.Fatal("Next timed out on primed snapshot")
	}

	e.Apply(func(g *graph.Graph) {
		g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
		g.AddIssue(graph.Issue{ID: 10, State: graph.Open})
		g.AddHierarchy(1, 10)
	})

	board, ok := sub.Next(ctx)
	if !ok {
		t.Fatal("Next timed out, want the post-Apply snapshot")
	}
	if len(board.GetEpics()) != 1 {
		t.Fatalf("post-Apply epics = %d, want 1", len(board.GetEpics()))
	}
}

func TestSubscribeReplaceLatestKeepsNewestSnapshot(t *testing.T) {
	e := New(nil, Config{})
	sub, cancel := e.Subscribe()
	defer cancel()

	// Two applies before the consumer reads: replace-latest must drop the
	// superseded snapshot and hand back only the newest state.
	e.Apply(func(g *graph.Graph) {
		g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
	})
	e.Apply(func(g *graph.Graph) {
		g.AddIssue(graph.Issue{ID: 10, State: graph.Open})
		g.AddHierarchy(1, 10)
	})

	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	board, ok := sub.Next(ctx)
	if !ok {
		t.Fatal("Next timed out")
	}
	// Newest state: epic #1 now has a ready child, so it is ACTIVE with one leaf.
	if len(board.GetEpics()) != 1 || len(board.GetEpics()[0].GetReady()) != 1 {
		t.Fatalf("got %d epics with %d ready, want newest snapshot (1 epic, 1 ready)",
			len(board.GetEpics()), len(board.GetEpics()[0].GetReady()))
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	e := New(nil, Config{})
	sub, cancel := e.Subscribe()

	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if _, ok := sub.Next(ctx); !ok {
		t.Fatal("Next timed out on primed snapshot")
	}
	cancel() // unregister

	// After cancel the engine no longer pushes to this subscriber; Next should
	// block until ctx expires rather than deliver an Apply.
	e.Apply(func(g *graph.Graph) {
		g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
	})
	short, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if _, ok := sub.Next(short); ok {
		t.Fatal("Next delivered after cancel, want no delivery")
	}
}

// TestConcurrentApplyAndSubscribe is the race-detector workout: many writers
// and readers hammering the engine at once must stay coherent.
func TestConcurrentApplyAndSubscribe(t *testing.T) {
	e := New(nil, Config{})

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id := graph.IssueID(base*100 + i)
				e.Apply(func(g *graph.Graph) {
					g.AddIssue(graph.Issue{ID: id, State: graph.Open, IsEpic: true})
				})
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, cancel := e.Subscribe()
			defer cancel()
			ctx, done := context.WithTimeout(context.Background(), time.Second)
			defer done()
			for i := 0; i < 20; i++ {
				if _, ok := sub.Next(ctx); !ok {
					return
				}
			}
		}()
	}
	wg.Wait()

	// All epics are childless, so each is EMPTY but still on the board.
	if n := len(e.Board().GetEpics()); n != 8*50 {
		t.Fatalf("epics = %d, want %d", n, 8*50)
	}
}
