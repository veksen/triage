package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
	"github.com/veksen/triage/gen/triage/v1/triagev1connect"
	"github.com/veksen/triage/internal/engine"
	"github.com/veksen/triage/internal/graph"
)

// newTestServer wires the handler over eng and returns a Connect client plus a
// teardown. The client talks the Connect protocol over HTTP/1.1, which is what
// the real TS client will do.
func newTestServer(t *testing.T, eng *engine.Engine) triagev1connect.TriageServiceClient {
	t.Helper()
	_, handler := triagev1connect.NewTriageServiceHandler(New(eng))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return triagev1connect.NewTriageServiceClient(srv.Client(), srv.URL)
}

func fixtureEngine() *engine.Engine {
	g := graph.NewGraph()
	g.AddIssue(graph.Issue{ID: 1, Title: "Ship dashboard", State: graph.Open, IsEpic: true})
	g.AddIssue(graph.Issue{ID: 10, Title: "Wire the API", State: graph.Open})
	g.AddHierarchy(1, 10)
	return engine.New(g, engine.Config{})
}

func TestGetBoardReturnsSnapshot(t *testing.T) {
	client := newTestServer(t, fixtureEngine())

	resp, err := client.GetBoard(context.Background(), connect.NewRequest(&triagev1.GetBoardRequest{}))
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if n := len(resp.Msg.GetBoard().GetEpics()); n != 1 {
		t.Fatalf("epics = %d, want 1", n)
	}
}

func TestStreamBoardPushesSnapshotThenUpdate(t *testing.T) {
	eng := engine.New(graph.NewGraph(), engine.Config{})
	client := newTestServer(t, eng)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamBoard(ctx, connect.NewRequest(&triagev1.StreamBoardRequest{}))
	if err != nil {
		t.Fatalf("StreamBoard: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	// First message: the current (empty) snapshot. Receiving it also guarantees
	// the handler has subscribed before we mutate below.
	if !stream.Receive() {
		t.Fatalf("first Receive failed: %v", stream.Err())
	}
	if n := len(stream.Msg().GetBoard().GetEpics()); n != 0 {
		t.Fatalf("first snapshot epics = %d, want 0", n)
	}

	eng.Apply(func(g *graph.Graph) {
		g.AddIssue(graph.Issue{ID: 1, State: graph.Open, IsEpic: true})
		g.AddIssue(graph.Issue{ID: 10, State: graph.Open})
		g.AddHierarchy(1, 10)
	})

	if !stream.Receive() {
		t.Fatalf("second Receive failed: %v", stream.Err())
	}
	if n := len(stream.Msg().GetBoard().GetEpics()); n != 1 {
		t.Fatalf("second snapshot epics = %d, want 1 after Apply", n)
	}
}
