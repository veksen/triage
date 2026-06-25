package sync

import (
	"context"
	"fmt"

	"github.com/veksen/triage/internal/github"
	"github.com/veksen/triage/internal/graph"
)

// Backfill builds the initial projection from GitHub: every open issue, plus the
// sub-issue and blocked-by edges among them. The returned graph is ready to hand
// to engine.New.
//
// Only open issues are inserted. That is intentional and correct: a closed
// blocker should not block (the algorithm treats absent/closed issues as
// satisfied), so dependency edges pointing at a closed issue resolve to "not
// blocking" for free, without special-casing here.
func Backfill(ctx context.Context, src Source, m github.Mapper) (*graph.Graph, error) {
	issues, err := src.ListOpenIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open issues: %w", err)
	}

	g := graph.NewGraph()
	for _, iss := range issues {
		m.UpsertIssue(iss)(g)
	}

	for _, iss := range issues {
		kids, err := src.ListSubIssues(ctx, iss.Number)
		if err != nil {
			return nil, fmt.Errorf("list sub-issues of #%d: %w", iss.Number, err)
		}
		for _, child := range kids {
			github.AddHierarchy(iss.Number, child)(g)
		}

		blockers, err := src.ListBlockedBy(ctx, iss.Number)
		if err != nil {
			return nil, fmt.Errorf("list blocked-by of #%d: %w", iss.Number, err)
		}
		for _, blocker := range blockers {
			github.AddDependency(iss.Number, blocker)(g)
		}
	}

	return g, nil
}
