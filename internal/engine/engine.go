// Package engine owns the in-memory projection and is the only sanctioned place
// the graph mutates. Every change recomputes the wire board and broadcasts a
// full snapshot to streaming subscribers, so consumers never reconcile deltas.
//
// The engine deliberately knows about the wire types (it produces *Board): it
// is the seam between the pure algorithm (internal/graph), the wire adapter
// (internal/api), and the transport (internal/server). Keeping recompute and
// broadcast here means the server layer is a thin shell with no business logic.
package engine

import (
	"sync"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
	"github.com/veksen/triage/internal/api"
	"github.com/veksen/triage/internal/graph"
)

// Config tunes a new Engine. The zero value is usable.
type Config struct {
	// RepoURL is the repository base (e.g. "https://github.com/veksen/triage")
	// used to build issue URLs. Empty omits URLs.
	RepoURL string
	// ParkedLabel is the label that parks an epic. Defaults to "parked".
	ParkedLabel string
	// Options tunes the selection algorithm. A nil Active predicate defaults to
	// graph.DefaultActive(ParkedLabel).
	Options graph.Options
}

// Engine holds the projection, the latest computed board, and the subscriber
// set. All three are guarded by a single mutex: recompute, broadcast, and
// subscriber registration are serialized so a subscriber can never miss the
// update that races its own registration.
type Engine struct {
	mu      sync.RWMutex
	g       *graph.Graph
	opts    graph.Options
	repoURL string
	board   *triagev1.Board

	subs   map[int]*subscriber
	nextID int
}

// New builds an Engine over an existing graph (nil means an empty graph) and
// computes the initial board. The graph is taken over by the engine; callers
// must mutate it only through Apply afterwards.
func New(g *graph.Graph, cfg Config) *Engine {
	if g == nil {
		g = graph.NewGraph()
	}
	parked := cfg.ParkedLabel
	if parked == "" {
		parked = "parked"
	}
	opts := cfg.Options
	if opts.Active == nil {
		opts.Active = graph.DefaultActive(parked)
	}
	e := &Engine{
		g:       g,
		opts:    opts,
		repoURL: cfg.RepoURL,
		subs:    map[int]*subscriber{},
	}
	e.board = e.compute()
	return e
}

// Board returns the latest computed snapshot. The returned board is immutable —
// the engine never mutates a board after publishing it — so callers may share
// it freely.
func (e *Engine) Board() *triagev1.Board {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.board
}

// Apply mutates the graph under the engine's lock, recomputes the board, and
// broadcasts the new snapshot. This is the only sanctioned way to change the
// projection — the sync layer drives the engine through it.
func (e *Engine) Apply(mutate func(g *graph.Graph)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	mutate(e.g)
	e.board = e.compute()
	e.broadcast(e.board)
}

// compute runs the selection and renders the wire board. The caller holds e.mu.
func (e *Engine) compute() *triagev1.Board {
	res := graph.Compute(e.g, e.opts)
	return api.NewBoardBuilder(e.g, e.repoURL).Board(res)
}

// broadcast pushes b to every subscriber. The caller holds e.mu; push is
// non-blocking (replace-latest), so holding the lock here is cheap and is what
// makes registration-vs-broadcast race-free.
func (e *Engine) broadcast(b *triagev1.Board) {
	for _, s := range e.subs {
		s.push(b)
	}
}
