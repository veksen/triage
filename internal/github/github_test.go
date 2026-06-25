package github

import (
	"testing"

	"github.com/veksen/triage/internal/graph"
)

// apply runs a batch of mutations against a fresh graph and returns it.
func apply(muts ...Mutation) *graph.Graph {
	g := graph.NewGraph()
	for _, m := range muts {
		m(g)
	}
	return g
}

func TestUpsertIssueMapsFields(t *testing.T) {
	m := NewMapper()
	g := apply(m.UpsertIssue(Issue{
		Number: 10, Title: "Wire the API", State: "open", Labels: []string{"backend"},
	}))

	iss, ok := g.Issue(10)
	if !ok {
		t.Fatal("issue 10 not in graph after upsert")
	}
	if iss.Title != "Wire the API" || iss.State != graph.Open || iss.IsEpic {
		t.Fatalf("mapped issue = %+v, want title set, Open, not epic", iss)
	}
}

func TestEpicDetectionByLabel(t *testing.T) {
	m := NewMapper() // default epic label
	g := apply(m.UpsertIssue(Issue{Number: 1, State: "open", Labels: []string{"epic"}}))
	if iss, _ := g.Issue(1); !iss.IsEpic {
		t.Fatal("issue with 'epic' label should map to IsEpic")
	}
}

func TestEpicDetectionByType(t *testing.T) {
	m := Mapper{EpicType: "Epic"} // honour native issue type instead of a label
	g := apply(m.UpsertIssue(Issue{Number: 1, State: "open", TypeName: "Epic"}))
	if iss, _ := g.Issue(1); !iss.IsEpic {
		t.Fatal("issue with native type 'Epic' should map to IsEpic when EpicType is set")
	}
}

func TestClosedStateMapping(t *testing.T) {
	g := apply(NewMapper().UpsertIssue(Issue{Number: 5, State: "closed"}))
	if iss, _ := g.Issue(5); iss.State != graph.Closed {
		t.Fatalf("state = %v, want Closed", iss.State)
	}
}

// --- webhook translation ---------------------------------------------------

func TestTranslateIssuesOpenedUpserts(t *testing.T) {
	body := []byte(`{"action":"opened","issue":{"number":10,"title":"T","state":"open","labels":[{"name":"epic"}]}}`)
	muts, err := NewMapper().Translate(EventIssues, body)
	if err != nil {
		t.Fatal(err)
	}
	g := apply(muts...)
	if iss, ok := g.Issue(10); !ok || !iss.IsEpic || iss.Title != "T" {
		t.Fatalf("issue 10 = %+v ok=%v, want epic titled T", iss, ok)
	}
}

func TestTranslateIssuesDeletedRemoves(t *testing.T) {
	g := apply(NewMapper().UpsertIssue(Issue{Number: 10, State: "open"}))
	muts, err := NewMapper().Translate(EventIssues, []byte(`{"action":"deleted","issue":{"number":10}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range muts {
		m(g)
	}
	if _, ok := g.Issue(10); ok {
		t.Fatal("issue 10 should be gone after a deleted event")
	}
}

func TestTranslateSubIssueAddedAndRemoved(t *testing.T) {
	added := `{"action":"sub_issue_added","parent_issue":{"number":1},"sub_issue":{"number":10}}`
	muts, err := NewMapper().Translate(EventSubIssues, []byte(added))
	if err != nil {
		t.Fatal(err)
	}
	// Seed the two issues, then apply the hierarchy edge.
	g := apply(
		NewMapper().UpsertIssue(Issue{Number: 1, State: "open", Labels: []string{"epic"}}),
		NewMapper().UpsertIssue(Issue{Number: 10, State: "open"}),
	)
	for _, m := range muts {
		m(g)
	}
	if got := graph.Compute(g, graph.Options{}); len(got.Epics) != 1 || got.Epics[0].Status != graph.StatusActive {
		t.Fatalf("after sub_issue_added want one ACTIVE epic, got %+v", got.Epics)
	}

	removed := `{"action":"sub_issue_removed","parent_issue":{"number":1},"sub_issue":{"number":10}}`
	rm, err := NewMapper().Translate(EventSubIssues, []byte(removed))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range rm {
		m(g)
	}
	if got := graph.Compute(g, graph.Options{}); got.Epics[0].Status != graph.StatusEmpty {
		t.Fatalf("after sub_issue_removed want EMPTY epic, got %+v", got.Epics[0])
	}
}

func TestTranslateDependencyAddedStalls(t *testing.T) {
	body := `{"action":"blocked_by_added","blocked_issue":{"number":10},"blocking_issue":{"number":99}}`
	muts, err := NewMapper().Translate(EventIssueDependencies, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	g := apply(
		NewMapper().UpsertIssue(Issue{Number: 1, State: "open", Labels: []string{"epic"}}),
		NewMapper().UpsertIssue(Issue{Number: 10, State: "open"}),
		NewMapper().UpsertIssue(Issue{Number: 99, State: "open"}),
		AddHierarchy(1, 10),
	)
	for _, m := range muts {
		m(g)
	}
	if got := graph.Compute(g, graph.Options{}).Epics[0].Status; got != graph.StatusStalled {
		t.Fatalf("status = %v after blocked_by_added, want STALLED", got)
	}
}

func TestTranslateUnknownEventIsNoOp(t *testing.T) {
	muts, err := NewMapper().Translate("push", []byte(`{}`))
	if err != nil {
		t.Fatalf("unknown event should not error, got %v", err)
	}
	if len(muts) != 0 {
		t.Fatalf("unknown event yielded %d mutations, want 0", len(muts))
	}
}

func TestTranslateMalformedBodyErrors(t *testing.T) {
	if _, err := NewMapper().Translate(EventIssues, []byte(`{not json`)); err == nil {
		t.Fatal("malformed issues payload should error")
	}
}
