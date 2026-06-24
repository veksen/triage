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
	mu          sync.RWMutex
	g           *graph.Graph
	opts        graph.Options
	repoURL     string
	parkedLabel string
	board       *triagev1.Board

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
		g:           g,
		opts:        opts,
		repoURL:     cfg.RepoURL,
		parkedLabel: parked,
		subs:        map[int]*subscriber{},
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

// SetEpicState drives (active=true) or parks (active=false) an epic by toggling
// the parked label on the projected issue, then recomputes and broadcasts. It
// returns the epic's resulting status and whether the epic exists; a parked
// epic leaves the board entirely and reports EPIC_STATUS_UNSPECIFIED.
//
// This mutates the in-memory projection only — it does NOT write back to GitHub
// (HANDOFF decision #6). A park therefore survives only until the next GitHub
// sync re-applies the issue; wiring write-back is an open decision for the sync
// phase.
func (e *Engine) SetEpicState(epic graph.IssueID, active bool) (triagev1.EpicStatus, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	iss, ok := e.g.Issue(epic)
	if !ok {
		return triagev1.EpicStatus_EPIC_STATUS_UNSPECIFIED, false
	}
	iss.Labels = setLabel(iss.Labels, e.parkedLabel, !active)
	e.g.AddIssue(iss)

	e.board = e.compute()
	e.broadcast(e.board)
	return statusInBoard(e.board, int64(epic)), true
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

// setLabel returns a copy of labels with label added or removed. A copy keeps
// the engine from mutating a slice the algorithm or a prior board may alias.
func setLabel(labels []string, label string, present bool) []string {
	out := make([]string, 0, len(labels)+1)
	found := false
	for _, l := range labels {
		if l == label {
			found = true
			if !present {
				continue
			}
		}
		out = append(out, l)
	}
	if present && !found {
		out = append(out, label)
	}
	return out
}

// statusInBoard finds an epic's status in a board, or UNSPECIFIED if the epic
// is not on the board (e.g. just parked).
func statusInBoard(b *triagev1.Board, epic int64) triagev1.EpicStatus {
	for _, ev := range b.GetEpics() {
		if ev.GetEpic().GetNumber() == epic {
			return ev.GetStatus()
		}
	}
	return triagev1.EpicStatus_EPIC_STATUS_UNSPECIFIED
}
