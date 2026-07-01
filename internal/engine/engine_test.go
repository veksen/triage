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
