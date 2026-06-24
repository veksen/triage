// Package server exposes the engine over Connect-RPC. It is a thin shell: every
// handler delegates straight to the engine, which owns recompute, broadcast,
// and concurrency. No business logic lives here.
package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
	"github.com/veksen/triage/gen/triage/v1/triagev1connect"
	"github.com/veksen/triage/internal/engine"
	"github.com/veksen/triage/internal/graph"
)

// Server implements triagev1connect.TriageServiceHandler over an Engine.
type Server struct {
	triagev1connect.UnimplementedTriageServiceHandler
	engine *engine.Engine
}

// New returns a Server backed by eng.
func New(eng *engine.Engine) *Server {
	return &Server{engine: eng}
}

// GetBoard returns the current snapshot.
func (s *Server) GetBoard(_ context.Context, _ *connect.Request[triagev1.GetBoardRequest]) (*connect.Response[triagev1.GetBoardResponse], error) {
	return connect.NewResponse(&triagev1.GetBoardResponse{Board: s.engine.Board()}), nil
}

// StreamBoard subscribes the caller to board snapshots. The first message is the
// current board; every subsequent message is a full snapshot pushed on change.
// It returns when the client disconnects (ctx done) or a send fails.
func (s *Server) StreamBoard(ctx context.Context, _ *connect.Request[triagev1.StreamBoardRequest], stream *connect.ServerStream[triagev1.StreamBoardResponse]) error {
	sub, cancel := s.engine.Subscribe()
	defer cancel()

	for {
		board, ok := sub.Next(ctx)
		if !ok {
			// ctx done: client hung up (or server shutting down). Surfacing the
			// cancellation cause is the conventional Connect stream close.
			return ctx.Err()
		}
		if err := stream.Send(&triagev1.StreamBoardResponse{Board: board}); err != nil {
			return err
		}
	}
}

// SetEpicState is the one write: drive or park an epic. The change touches the
// in-memory projection only (see engine.SetEpicState). Unknown epics are a
// NotFound error.
func (s *Server) SetEpicState(_ context.Context, req *connect.Request[triagev1.SetEpicStateRequest]) (*connect.Response[triagev1.SetEpicStateResponse], error) {
	msg := req.Msg
	status, ok := s.engine.SetEpicState(graph.IssueID(msg.GetEpicNumber()), msg.GetActive())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("epic #%d not found", msg.GetEpicNumber()))
	}
	return connect.NewResponse(&triagev1.SetEpicStateResponse{Status: status}), nil
}
