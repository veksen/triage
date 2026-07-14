package sync

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/veksen/triage/internal/github"
	"github.com/veksen/triage/internal/graph"
)

// backfillWorkers bounds concurrent GitHub reads. Real repos have hundreds of
// open issues, each needing a sub-issues and a blocked-by fetch, so serial reads
// blow the startup budget; a small pool keeps it fast without tripping GitHub's
// secondary (burst) rate limits.
const backfillWorkers = 6

// Backfill builds the initial projection from GitHub: every open issue, plus the
// sub-issue and blocked-by edges among them. The returned graph is ready to hand
// to engine.New.
//
// Only open issues are inserted. That is intentional and correct: a closed
// blocker should not block (the algorithm treats absent/closed issues as
// satisfied), so dependency edges pointing at a closed issue resolve to "not
// blocking" for free, without special-casing here.
//
// Per-issue edge fetches run concurrently and are fault-tolerant: a single
// issue's failed sub-issues/blocked-by read is logged and skipped rather than
// aborting the whole backfill, so one bad issue never blanks the board. The
// graph itself is mutated by this goroutine only (it is not concurrency-safe);
// workers just return data.
func Backfill(ctx context.Context, src Source, m github.Mapper) (*graph.Graph, error) {
	issues, err := src.ListOpenIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open issues: %w", err)
	}

	g := graph.NewGraph()
	for _, iss := range issues {
		m.UpsertIssue(iss)(g)
	}

	type edges struct {
		issue    int
		kids     []int
		blockers []int
		failed   int // 0, 1, or 2 of the two reads failed
	}

	jobs := make(chan github.Issue)
	results := make(chan edges)
	var wg sync.WaitGroup
	for i := 0; i < backfillWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iss := range jobs {
				e := edges{issue: iss.Number}
				if kids, err := src.ListSubIssues(ctx, iss.Number); err != nil {
					e.failed++
				} else {
					e.kids = kids
				}
				if blockers, err := src.ListBlockedBy(ctx, iss.Number); err != nil {
					e.failed++
				} else {
					e.blockers = blockers
				}
				results <- e
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, iss := range issues {
			select {
			case jobs <- iss:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var failed int
	for r := range results {
		failed += r.failed
		for _, child := range r.kids {
			github.AddHierarchy(r.issue, child)(g)
		}
		for _, blocker := range r.blockers {
			github.AddDependency(r.issue, blocker)(g)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("backfill interrupted after %d issues: %w", len(issues), err)
	}
	if failed > 0 {
		log.Printf("backfill: %d edge fetches failed and were skipped (of %d issues)", failed, len(issues))
	}
	return g, nil
}
