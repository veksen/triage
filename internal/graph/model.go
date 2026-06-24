// Package graph holds the in-memory projection of a GitHub issue graph and the
// node-selection logic that turns "hundreds of open issues" into "the handful
// you can act on right now".
//
// The graph has two kinds of edges:
//
//   - hierarchy edges: parent -> sub-issue (epic -> story -> task), the "ladder"
//   - dependency edges: "blocked by" / "blocking"
//
// Nothing here talks to GitHub. The sync layer builds a Graph; this package
// reads it. Keeping the algorithm pure makes it fully testable against fixture
// graphs, which is the whole point of the first deliverable.
package graph

import "sort"

// IssueID is a GitHub issue number, unique within a repository.
type IssueID int

// State is an issue's open/closed state.
type State int

const (
	// Open issues are live work. Only open issues ever render.
	Open State = iota
	// Closed issues are done; they prune completed branches from the view.
	Closed
)

// Issue is a single GitHub issue projected into the graph. It carries only the
// fields the selection algorithm needs; the sync layer populates them.
type Issue struct {
	ID     IssueID
	Title  string
	State  State
	Labels []string

	// IsEpic marks an issue as an epic — the driving unit of the board.
	// How "epic" is determined (issue type, label, …) is the sync layer's
	// concern, not this package's.
	IsEpic bool

	// Priority is an optional ranking weight for epics (higher wins). It only
	// matters for epics; task-level priority is expressed through leverage.
	Priority int
}

// HasLabel reports whether the issue carries the given label.
func (i Issue) HasLabel(label string) bool {
	for _, l := range i.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// isOpen is a small convenience used throughout the algorithm.
func (i Issue) isOpen() bool { return i.State == Open }

// Graph is the cached projection: issues plus the two edge types, indexed for
// the traversals the algorithm performs.
type Graph struct {
	issues map[IssueID]Issue

	// children maps a parent to its sub-issues (hierarchy, downward).
	children map[IssueID][]IssueID
	// parent maps a sub-issue to its single parent (hierarchy, upward), used
	// to walk ancestry for the "why does this node exist" context.
	parent map[IssueID]IssueID

	// blockedBy maps an issue to the issues that block it (it cannot start
	// until they close).
	blockedBy map[IssueID][]IssueID
	// blocks maps an issue to the issues it blocks — the downstream used to
	// measure leverage.
	blocks map[IssueID][]IssueID
}

// NewGraph returns an empty graph ready to be populated.
func NewGraph() *Graph {
	return &Graph{
		issues:    map[IssueID]Issue{},
		children:  map[IssueID][]IssueID{},
		parent:    map[IssueID]IssueID{},
		blockedBy: map[IssueID][]IssueID{},
		blocks:    map[IssueID][]IssueID{},
	}
}

// AddIssue inserts or replaces an issue. Replacing is intentional: the sync
// layer re-applies issues as GitHub events arrive.
func (g *Graph) AddIssue(i Issue) { g.issues[i.ID] = i }

// AddHierarchy records that child is a sub-issue of parent. A sub-issue has at
// most one parent (GitHub's model); the last writer wins.
func (g *Graph) AddHierarchy(parent, child IssueID) {
	g.children[parent] = appendUnique(g.children[parent], child)
	g.parent[child] = parent
}

// AddDependency records that blocked is "blocked by" blocker — i.e. blocker
// must close before blocked can start.
func (g *Graph) AddDependency(blocked, blocker IssueID) {
	g.blockedBy[blocked] = appendUnique(g.blockedBy[blocked], blocker)
	g.blocks[blocker] = appendUnique(g.blocks[blocker], blocked)
}

// Issue returns the issue and whether it exists.
func (g *Graph) Issue(id IssueID) (Issue, bool) {
	i, ok := g.issues[id]
	return i, ok
}

// isOpen reports whether an issue exists and is open. Unknown issues count as
// not-open so dangling references never appear as actionable work.
func (g *Graph) isOpen(id IssueID) bool {
	i, ok := g.issues[id]
	return ok && i.isOpen()
}

// isReady reports whether an open issue has no open blocker — the "go do this
// now" condition.
func (g *Graph) isReady(id IssueID) bool {
	if !g.isOpen(id) {
		return false
	}
	for _, b := range g.blockedBy[id] {
		if g.isOpen(b) {
			return false
		}
	}
	return true
}

// ActivePredicate decides whether an epic is currently driving the board. It is
// the single human lever, kept as one pure function so its polarity (opt-in vs
// opt-out) is a configuration detail, not baked into the algorithm.
type ActivePredicate func(Issue) bool

// DefaultActive implements "free play by default": every open epic drives the
// board unless it carries the parked label. Closing or parking an epic removes
// its whole ladder automatically — the graph self-cleans, so a stale epic can
// never silently re-clutter the board.
func DefaultActive(parkedLabel string) ActivePredicate {
	return func(i Issue) bool {
		return i.IsEpic && i.isOpen() && !i.HasLabel(parkedLabel)
	}
}

// ActiveEpics returns the IDs of epics the predicate marks active, sorted for
// deterministic output.
func (g *Graph) ActiveEpics(pred ActivePredicate) []IssueID {
	var out []IssueID
	for id, iss := range g.issues {
		if pred(iss) {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func appendUnique(s []IssueID, v IssueID) []IssueID {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
