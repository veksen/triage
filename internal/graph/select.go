package graph

import "sort"

// This file implements the node-selection algorithm from the design doc (§5).
//
// Interpreting the spec's scope/ready/stall steps required one deliberate
// disambiguation, recorded here because it shapes everything below.
//
// scope(e) is defined as (a) the epic's open hierarchy descendants — its
// "ladder" — UNION (b) the transitive open blockers of that ladder. We keep
// those two subsets distinct:
//
//   - The READY FRONTIER (the "what's next" answer) is drawn from the LADDER
//     only. These are issues that directly advance the epic.
//   - The BLOCKER CLOSURE is carried in scope so we can render and reason about
//     prerequisites, but a ready blocker is not "what's next for the epic" — it
//     is "what's holding the epic up".
//
// Why not treat every ready in-scope issue as the frontier (the most literal
// reading of step 2)? Because the blocker closure of any finite DAG always
// bottoms out in ready roots, so "no ready issue in scope" would essentially
// never fire and stall detection (step 3) would be dead code. Splitting ladder
// from blockers is what makes "ready leaves = what's next; the blocking node
// when nothing's ready = why we're stuck" actually hold.

// Status describes how an active epic is doing.
type Status int

const (
	// StatusActive: the epic has ready work to pick up.
	StatusActive Status = iota
	// StatusStalled: the epic has open work but all of it is blocked; the
	// actionable blockers are surfaced instead.
	StatusStalled
	// StatusEmpty: the epic has no open ladder work at all (done or unplanned).
	StatusEmpty
)

// Node is a rendered issue with the context and flags the frontend needs.
type Node struct {
	ID IssueID
	// Leverage is how many open in-scope issues this node transitively unblocks.
	Leverage int
	// Ancestry is the hierarchy path toward the epic, nearest-first, capped to
	// the configured depth, so every node carries its "why".
	Ancestry []IssueID
	// ServesEpics lists every active epic whose scope includes this node. More
	// than one is the high-value DAG case, flagged via MultiEpic.
	ServesEpics []IssueID
	// MultiEpic is true when the node serves more than one active epic.
	MultiEpic bool
	// HighLeverage is true when Leverage meets the configured threshold.
	HighLeverage bool
}

// EpicView is the per-epic result: ready leaves when active, blockers when
// stalled.
type EpicView struct {
	Epic     IssueID
	Status   Status
	Ready    []Node // ranked, capped to TopN (StatusActive)
	Blockers []Node // actionable open blockers (StatusStalled)
}

// Result is the full render set: one view per active epic. An empty active set
// yields an empty result — that is correct, not an error.
type Result struct {
	Epics []EpicView
}

// Options tune the render without touching the core logic. The zero value is
// usable; missing fields fall back to sensible defaults.
type Options struct {
	// Active selects the driving epics. Defaults to DefaultActive("parked").
	Active ActivePredicate
	// AncestryDepth is how many hierarchy levels of context to attach to each
	// node (clamped to 1..3). Defaults to 2.
	AncestryDepth int
	// TopN caps ready leaves shown per epic; the rest are scrollable. 0 means
	// unlimited. Defaults to 5.
	TopN int
	// HighLeverageThreshold is the leverage at which a node is flagged
	// high-leverage. Defaults to 3.
	HighLeverageThreshold int
}

func (o Options) withDefaults() Options {
	if o.Active == nil {
		o.Active = DefaultActive("parked")
	}
	if o.AncestryDepth == 0 {
		o.AncestryDepth = 2
	}
	if o.AncestryDepth < 1 {
		o.AncestryDepth = 1
	}
	if o.AncestryDepth > 3 {
		o.AncestryDepth = 3
	}
	if o.TopN == 0 {
		o.TopN = 5
	}
	if o.HighLeverageThreshold == 0 {
		o.HighLeverageThreshold = 3
	}
	return o
}

// scopeInfo is one epic's induced subgraph: its ladder and its blocker closure.
type scopeInfo struct {
	ladder   map[IssueID]bool // open hierarchy descendants (the epic's own work)
	blockers map[IssueID]bool // open transitive blockers of the ladder, ladder excluded
}

// Compute runs the full selection: scope -> ready frontier -> stall detection ->
// rank -> render set, for the currently-active epics.
func Compute(g *Graph, opts Options) Result {
	opts = opts.withDefaults()
	active := g.ActiveEpics(opts.Active)

	// 1. SCOPE — per epic, plus the union for cross-epic membership/leverage.
	scopes := make(map[IssueID]scopeInfo, len(active))
	inScope := map[IssueID]bool{}
	for _, e := range active {
		si := g.scopeOf(e)
		scopes[e] = si
		for id := range si.ladder {
			inScope[id] = true
		}
		for id := range si.blockers {
			inScope[id] = true
		}
	}

	res := Result{Epics: make([]EpicView, 0, len(active))}
	for _, e := range active {
		res.Epics = append(res.Epics, g.buildEpicView(e, scopes, inScope, opts))
	}
	return res
}

// scopeOf computes scope(e): open hierarchy descendants (ladder) and the
// transitive open blockers of that ladder (blocker closure, ladder excluded).
func (g *Graph) scopeOf(epic IssueID) scopeInfo {
	si := scopeInfo{ladder: map[IssueID]bool{}, blockers: map[IssueID]bool{}}

	// (a) ladder: walk hierarchy down from the epic, collecting open issues.
	// We traverse through closed intermediates (a closed story may still have
	// open children worth doing) but only collect open nodes; the epic itself
	// is a container, not ladder work.
	seen := map[IssueID]bool{epic: true}
	stack := append([]IssueID(nil), g.children[epic]...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		if g.isOpen(id) {
			si.ladder[id] = true
		}
		stack = append(stack, g.children[id]...)
	}

	// (b) blocker closure: from every ladder issue, walk blocked-by edges,
	// collecting open issues not already in the ladder.
	bseen := map[IssueID]bool{}
	var bstack []IssueID
	for id := range si.ladder {
		bstack = append(bstack, g.blockedBy[id]...)
	}
	for len(bstack) > 0 {
		id := bstack[len(bstack)-1]
		bstack = bstack[:len(bstack)-1]
		if bseen[id] {
			continue
		}
		bseen[id] = true
		if g.isOpen(id) && !si.ladder[id] {
			si.blockers[id] = true
		}
		bstack = append(bstack, g.blockedBy[id]...)
	}
	return si
}

// buildEpicView assembles one epic's view: ready frontier or stall + blockers.
func (g *Graph) buildEpicView(epic IssueID, scopes map[IssueID]scopeInfo, inScope map[IssueID]bool, opts Options) EpicView {
	si := scopes[epic]
	view := EpicView{Epic: epic}

	// No open ladder work — nothing to drive.
	if len(si.ladder) == 0 {
		view.Status = StatusEmpty
		return view
	}

	// 2. READY FRONTIER — ready issues within the ladder.
	var ready []IssueID
	for id := range si.ladder {
		if g.isReady(id) {
			ready = append(ready, id)
		}
	}

	if len(ready) > 0 {
		view.Status = StatusActive
		view.Ready = g.rankNodes(ready, epic, scopes, inScope, opts)
		if opts.TopN > 0 && len(view.Ready) > opts.TopN {
			view.Ready = view.Ready[:opts.TopN]
		}
		return view
	}

	// 3. STALL DETECTION — ladder is fully blocked. Surface the actionable
	// (ready) blockers: the open prerequisites that, once resolved, start to
	// unblock the ladder. These may be out of the epic's scope — that's the
	// high-value cross-epic signal, not a bug.
	view.Status = StatusStalled
	var blockers []IssueID
	for id := range si.blockers {
		if g.isReady(id) {
			blockers = append(blockers, id)
		}
	}
	view.Blockers = g.rankNodes(blockers, epic, scopes, inScope, opts)
	return view
}

// rankNodes turns issue IDs into ranked, flagged Nodes. Ranking: leverage
// descending, then ID ascending for stable output. (Epic priority orders epics,
// which is the caller's concern; within one epic it is constant.)
func (g *Graph) rankNodes(ids []IssueID, epic IssueID, scopes map[IssueID]scopeInfo, inScope map[IssueID]bool, opts Options) []Node {
	nodes := make([]Node, 0, len(ids))
	for _, id := range ids {
		lev := g.leverage(id, inScope)
		serves := servingEpics(id, scopes)
		nodes = append(nodes, Node{
			ID:           id,
			Leverage:     lev,
			Ancestry:     g.ancestryToward(id, epic, opts.AncestryDepth),
			ServesEpics:  serves,
			MultiEpic:    len(serves) > 1,
			HighLeverage: lev >= opts.HighLeverageThreshold,
		})
	}
	sort.Slice(nodes, func(a, b int) bool {
		if nodes[a].Leverage != nodes[b].Leverage {
			return nodes[a].Leverage > nodes[b].Leverage
		}
		return nodes[a].ID < nodes[b].ID
	})
	return nodes
}

// leverage counts the open in-scope issues this node transitively unblocks
// (downstream over dependency edges), excluding itself.
func (g *Graph) leverage(id IssueID, inScope map[IssueID]bool) int {
	seen := map[IssueID]bool{id: true}
	stack := append([]IssueID(nil), g.blocks[id]...)
	count := 0
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		if inScope[n] && g.isOpen(n) {
			count++
		}
		stack = append(stack, g.blocks[n]...)
	}
	return count
}

// servingEpics lists the active epics whose scope (ladder or blocker closure)
// contains the node, sorted for determinism. This is how the frontend dedupes a
// shared node into one rendered vertex while still showing every epic it serves.
func servingEpics(id IssueID, scopes map[IssueID]scopeInfo) []IssueID {
	var out []IssueID
	for epic, si := range scopes {
		if si.ladder[id] || si.blockers[id] {
			out = append(out, epic)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

// ancestryToward walks hierarchy parents from node up toward epic, nearest
// first, stopping at the epic or after depth levels. Best-effort: a cross-epic
// blocker may have no path to this epic, yielding a shorter or empty trail.
func (g *Graph) ancestryToward(node, epic IssueID, depth int) []IssueID {
	var path []IssueID
	seen := map[IssueID]bool{node: true}
	cur := node
	for len(path) < depth {
		p, ok := g.parent[cur]
		if !ok || seen[p] {
			break
		}
		seen[p] = true
		path = append(path, p)
		if p == epic {
			break
		}
		cur = p
	}
	return path
}
