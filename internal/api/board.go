// Package api adapts the pure graph types into the Connect wire types. Keeping
// this mapping out of the graph package lets the algorithm stay transport- and
// proto-agnostic; this layer enriches bare issue IDs with the display fields
// (title, URL, open state) the frontend needs.
package api

import (
	"fmt"

	triagev1 "github.com/veksen/triage/gen/triage/v1"
	"github.com/veksen/triage/internal/graph"
)

// BoardBuilder turns a graph.Result into a *triagev1.Board. repoURL is the
// repository base (e.g. "https://github.com/veksen/triage"); when empty, issue
// URLs are omitted.
type BoardBuilder struct {
	g       *graph.Graph
	repoURL string
}

// NewBoardBuilder returns a builder bound to a graph snapshot.
func NewBoardBuilder(g *graph.Graph, repoURL string) *BoardBuilder {
	return &BoardBuilder{g: g, repoURL: repoURL}
}

// Board maps a full selection result to the wire board.
func (b *BoardBuilder) Board(res graph.Result) *triagev1.Board {
	epics := make([]*triagev1.EpicView, 0, len(res.Epics))
	for _, ev := range res.Epics {
		epics = append(epics, &triagev1.EpicView{
			Epic:     b.issueRef(ev.Epic),
			Status:   toStatus(ev.Status),
			Ready:    b.nodes(ev.Ready),
			Blockers: b.nodes(ev.Blockers),
		})
	}
	return &triagev1.Board{Epics: epics}
}

func (b *BoardBuilder) nodes(ns []graph.Node) []*triagev1.Node {
	if len(ns) == 0 {
		return nil
	}
	out := make([]*triagev1.Node, 0, len(ns))
	for _, n := range ns {
		out = append(out, &triagev1.Node{
			Issue:        b.issueRef(n.ID),
			Leverage:     int32(n.Leverage),
			Ancestry:     b.issueRefs(n.Ancestry),
			ServesEpics:  toInt64s(n.ServesEpics),
			MultiEpic:    n.MultiEpic,
			HighLeverage: n.HighLeverage,
		})
	}
	return out
}

func (b *BoardBuilder) issueRefs(ids []graph.IssueID) []*triagev1.IssueRef {
	if len(ids) == 0 {
		return nil
	}
	out := make([]*triagev1.IssueRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, b.issueRef(id))
	}
	return out
}

func (b *BoardBuilder) issueRef(id graph.IssueID) *triagev1.IssueRef {
	ref := &triagev1.IssueRef{Number: int64(id)}
	if iss, ok := b.g.Issue(id); ok {
		ref.Title = iss.Title
		ref.Open = iss.State == graph.Open
	}
	if b.repoURL != "" {
		ref.Url = fmt.Sprintf("%s/issues/%d", b.repoURL, int64(id))
	}
	return ref
}

func toStatus(s graph.Status) triagev1.EpicStatus {
	switch s {
	case graph.StatusActive:
		return triagev1.EpicStatus_EPIC_STATUS_ACTIVE
	case graph.StatusStalled:
		return triagev1.EpicStatus_EPIC_STATUS_STALLED
	case graph.StatusEmpty:
		return triagev1.EpicStatus_EPIC_STATUS_EMPTY
	default:
		return triagev1.EpicStatus_EPIC_STATUS_UNSPECIFIED
	}
}

func toInt64s(ids []graph.IssueID) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		out = append(out, int64(id))
	}
	return out
}
