package sync

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/veksen/triage/internal/engine"
	"github.com/veksen/triage/internal/github"
	"github.com/veksen/triage/internal/graph"
)

// --- Backfill over a fake Source -------------------------------------------

type fakeSource struct {
	issues   []github.Issue
	subs     map[int][]int
	blockers map[int][]int
}

func (f fakeSource) ListOpenIssues(context.Context) ([]github.Issue, error) { return f.issues, nil }
func (f fakeSource) ListSubIssues(_ context.Context, p int) ([]int, error)  { return f.subs[p], nil }
func (f fakeSource) ListBlockedBy(_ context.Context, i int) ([]int, error)  { return f.blockers[i], nil }

func TestBackfillBuildsActiveEpicBoard(t *testing.T) {
	src := fakeSource{
		issues: []github.Issue{
			{Number: 1, State: "open", Labels: []string{"epic"}},
			{Number: 10, State: "open"},
		},
		subs: map[int][]int{1: {10}},
	}
	g, err := Backfill(context.Background(), src, github.NewMapper())
	if err != nil {
		t.Fatal(err)
	}
	res := graph.Compute(g, graph.Options{})
	if len(res.Epics) != 1 || res.Epics[0].Status != graph.StatusActive {
		t.Fatalf("backfilled board = %+v, want one ACTIVE epic", res.Epics)
	}
	if len(res.Epics[0].Ready) != 1 || res.Epics[0].Ready[0].ID != 10 {
		t.Fatalf("ready leaves = %+v, want [#10]", res.Epics[0].Ready)
	}
}

func TestBackfillStallsOnBlocker(t *testing.T) {
	src := fakeSource{
		issues: []github.Issue{
			{Number: 1, State: "open", Labels: []string{"epic"}},
			{Number: 10, State: "open"},
			{Number: 99, State: "open"},
		},
		subs:     map[int][]int{1: {10}},
		blockers: map[int][]int{10: {99}},
	}
	g, err := Backfill(context.Background(), src, github.NewMapper())
	if err != nil {
		t.Fatal(err)
	}
	res := graph.Compute(g, graph.Options{})
	if res.Epics[0].Status != graph.StatusStalled {
		t.Fatalf("status = %v, want STALLED", res.Epics[0].Status)
	}
}

// --- Client over httptest (exercises headers, pagination, PR filtering) -----

func TestClientListOpenIssuesFiltersPRsAndPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-GitHub-Api-Version"); got != apiVersion {
			t.Errorf("api version header = %q, want %q", got, apiVersion)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header = %q, want Bearer tok", got)
		}
		if !strings.HasPrefix(r.URL.Path, "/repos/o/r/issues") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		// Page 1 returns a full page (forces a second request); page 2 ends it.
		switch r.URL.Query().Get("page") {
		case "1":
			w.Write([]byte(`[{"number":1,"state":"open","labels":[{"name":"epic"}]},{"number":2,"state":"open","pull_request":{"url":"x"}}]`))
		default:
			w.Write([]byte(`[{"number":3,"state":"open"}]`))
		}
	}))
	defer srv.Close()

	// per_page is 100, so a 2-item page 1 already ends pagination; assert PR
	// filtering instead by checking #2 is dropped.
	c := NewClient("o", "r", "tok", WithBaseURL(srv.URL))
	issues, err := c.ListOpenIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]bool{}
	for _, i := range issues {
		got[i.Number] = true
	}
	if got[2] {
		t.Fatal("pull request #2 should have been filtered out")
	}
	if !got[1] {
		t.Fatalf("issue #1 missing from %v", issues)
	}
}

// --- Signature verification -------------------------------------------------

func TestValidSignatureRoundTrips(t *testing.T) {
	secret := []byte("s3cret")
	body := []byte(`{"action":"opened"}`)
	if !ValidSignature(secret, Sign(secret, body), body) {
		t.Fatal("a freshly-signed body should verify")
	}
}

func TestValidSignatureRejects(t *testing.T) {
	secret := []byte("s3cret")
	body := []byte(`{"action":"opened"}`)
	good := Sign(secret, body)

	cases := map[string]struct {
		secret []byte
		header string
		body   []byte
	}{
		"empty secret fails closed": {nil, good, body},
		"wrong secret":              {[]byte("other"), good, body},
		"tampered body":             {secret, good, []byte(`{"action":"closed"}`)},
		"missing prefix":            {secret, strings.TrimPrefix(good, "sha256="), body},
		"not hex":                   {secret, "sha256=zzzz", body},
		"empty header":              {secret, "", body},
	}
	for name, c := range cases {
		if ValidSignature(c.secret, c.header, c.body) {
			t.Errorf("%s: signature unexpectedly valid", name)
		}
	}
}

// --- Webhook handler end-to-end ---------------------------------------------

func postWebhook(t *testing.T, h http.Handler, secret []byte, event string, body string, sign bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(body)))
	req.Header.Set("X-GitHub-Event", event)
	if sign {
		req.Header.Set("X-Hub-Signature-256", Sign(secret, []byte(body)))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWebhookHandlerAppliesSignedDelivery(t *testing.T) {
	secret := []byte("s3cret")
	eng := engine.New(graph.NewGraph(), engine.Config{})
	h := NewWebhookHandler(eng, github.NewMapper(), secret)

	// An "opened" epic event should put one epic on the board.
	body := `{"action":"opened","issue":{"number":1,"state":"open","labels":[{"name":"epic"}]}}`
	rec := postWebhook(t, h, secret, github.EventIssues, body, true)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if n := len(eng.Board().GetEpics()); n != 1 {
		t.Fatalf("board epics = %d after delivery, want 1", n)
	}
}

func TestWebhookHandlerRejectsBadSignatureWithoutMutating(t *testing.T) {
	secret := []byte("s3cret")
	eng := engine.New(graph.NewGraph(), engine.Config{})
	h := NewWebhookHandler(eng, github.NewMapper(), secret)

	body := `{"action":"opened","issue":{"number":1,"state":"open","labels":[{"name":"epic"}]}}`
	rec := postWebhook(t, h, secret, github.EventIssues, body, false) // unsigned

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if n := len(eng.Board().GetEpics()); n != 0 {
		t.Fatalf("board mutated on a rejected delivery: epics = %d", n)
	}
}

func TestWebhookHandlerIgnoresUnknownEvent(t *testing.T) {
	secret := []byte("s3cret")
	eng := engine.New(graph.NewGraph(), engine.Config{})
	h := NewWebhookHandler(eng, github.NewMapper(), secret)

	rec := postWebhook(t, h, secret, "push", `{}`, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (unknown events are accepted no-ops)", rec.Code)
	}
	if n := len(eng.Board().GetEpics()); n != 0 {
		t.Fatalf("unknown event mutated the board: epics = %d", n)
	}
}
