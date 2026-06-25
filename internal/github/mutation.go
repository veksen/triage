// Package github translates GitHub's data — REST issues and webhook payloads —
// into graph mutations. It is deliberately pure: no network, no go-github types,
// no HTTP. The network/HTTP shell lives in internal/sync and decodes GitHub JSON
// into the lean structs here, so every mapping below is unit-testable against
// fixtures the way internal/graph is.
package github

import "github.com/veksen/triage/internal/graph"

// Mutation is a single change to the projection. It matches engine.Apply's
// argument exactly, so internal/sync applies a webhook's mutations with
// engine.Apply and a backfill batch with one loop inside a single Apply.
type Mutation = func(*graph.Graph)

// Issue is the slice of a GitHub issue this dashboard projects. Numbers are
// per-repository issue numbers (NOT the global database id) — the graph keys on
// them, so the decoders must read GitHub's "number" field, never an "*_id".
type Issue struct {
	Number   int
	Title    string
	State    string // "open"; anything else is treated as closed
	Labels   []string
	TypeName string // GitHub native issue type name, if any (e.g. "Epic")
}

// DefaultEpicLabel marks an issue as an epic by default. It mirrors the parked
// lever's mechanism (a label), so "what drives the board" is one click.
const DefaultEpicLabel = "epic"

// Mapper turns GitHub structures into graph mutations. EpicLabel and EpicType
// are the configurable answer to the still-open "what marks an epic" decision
// (HANDOFF §9): an issue is an epic if it carries EpicLabel or, when EpicType is
// set, if its native issue type matches. The default is label-only; set
// EpicType to also (or instead) honour GitHub issue types.
type Mapper struct {
	EpicLabel string
	EpicType  string
}

// NewMapper returns a Mapper using the default epic label.
func NewMapper() Mapper { return Mapper{EpicLabel: DefaultEpicLabel} }

// ToIssue projects a GitHub issue into a graph.Issue.
func (m Mapper) ToIssue(i Issue) graph.Issue {
	return graph.Issue{
		ID:     graph.IssueID(i.Number),
		Title:  i.Title,
		State:  toState(i.State),
		Labels: i.Labels,
		IsEpic: m.isEpic(i),
	}
}

// UpsertIssue is the mutation that inserts or replaces an issue. The same call
// handles open/close, label changes (incl. park/unpark), and edits, because the
// payload always carries the issue's current state.
func (m Mapper) UpsertIssue(i Issue) Mutation {
	iss := m.ToIssue(i)
	return func(g *graph.Graph) { g.AddIssue(iss) }
}

func (m Mapper) isEpic(i Issue) bool {
	label := m.EpicLabel
	if label == "" {
		label = DefaultEpicLabel
	}
	for _, l := range i.Labels {
		if l == label {
			return true
		}
	}
	return m.EpicType != "" && i.TypeName == m.EpicType
}

// AddHierarchy links parent -> child (parent's sub-issue).
func AddHierarchy(parent, child int) Mutation {
	return func(g *graph.Graph) { g.AddHierarchy(graph.IssueID(parent), graph.IssueID(child)) }
}

// RemoveHierarchy unlinks parent -> child.
func RemoveHierarchy(parent, child int) Mutation {
	return func(g *graph.Graph) { g.RemoveHierarchy(graph.IssueID(parent), graph.IssueID(child)) }
}

// AddDependency records that blocked is blocked by blocker.
func AddDependency(blocked, blocker int) Mutation {
	return func(g *graph.Graph) { g.AddDependency(graph.IssueID(blocked), graph.IssueID(blocker)) }
}

// RemoveDependency drops the blocked/blocker edge.
func RemoveDependency(blocked, blocker int) Mutation {
	return func(g *graph.Graph) { g.RemoveDependency(graph.IssueID(blocked), graph.IssueID(blocker)) }
}

// RemoveIssue deletes an issue and all its edges (the issues "deleted" webhook).
func RemoveIssue(number int) Mutation {
	return func(g *graph.Graph) { g.RemoveIssue(graph.IssueID(number)) }
}

func toState(s string) graph.State {
	if s == "open" {
		return graph.Open
	}
	return graph.Closed
}
