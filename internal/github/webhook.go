package github

import (
	"encoding/json"
	"fmt"
)

// Webhook event names (the X-GitHub-Event header), verified against GitHub's
// docs (API version 2026-03-10).
const (
	EventIssues            = "issues"
	EventSubIssues         = "sub_issues"
	EventIssueDependencies = "issue_dependencies"
)

// wireIssue is the slice of GitHub's webhook "issue" object we read. Crucially
// we read number (per-repo), not id (global), since the graph keys on number.
type wireIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Type *struct {
		Name string `json:"name"`
	} `json:"type"`
}

func (w wireIssue) toIssue() Issue {
	labels := make([]string, 0, len(w.Labels))
	for _, l := range w.Labels {
		labels = append(labels, l.Name)
	}
	iss := Issue{Number: w.Number, Title: w.Title, State: w.State, Labels: labels}
	if w.Type != nil {
		iss.TypeName = w.Type.Name
	}
	return iss
}

// Translate decodes one webhook delivery (its X-GitHub-Event value and raw body)
// into the graph mutations it implies. Unrecognised events yield no mutations
// and no error, so a broad webhook subscription is harmless. A recognised event
// with an unhandled action (e.g. an issue "assigned") also yields nothing.
func (m Mapper) Translate(event string, body []byte) ([]Mutation, error) {
	switch event {
	case EventIssues:
		return m.translateIssues(body)
	case EventSubIssues:
		return translateSubIssues(body)
	case EventIssueDependencies:
		return translateDependencies(body)
	default:
		return nil, nil
	}
}

func (m Mapper) translateIssues(body []byte) ([]Mutation, error) {
	var p struct {
		Action string    `json:"action"`
		Issue  wireIssue `json:"issue"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode issues payload: %w", err)
	}
	switch p.Action {
	case "deleted", "transferred":
		// Transfer moves the issue to another repo; locally it is gone.
		return []Mutation{RemoveIssue(p.Issue.Number)}, nil
	default:
		// opened, edited, closed, reopened, labeled, unlabeled, … all carry the
		// issue's current state, so a single upsert keeps the projection correct.
		return []Mutation{m.UpsertIssue(p.Issue.toIssue())}, nil
	}
}

func translateSubIssues(body []byte) ([]Mutation, error) {
	var p struct {
		Action      string    `json:"action"`
		SubIssue    wireIssue `json:"sub_issue"`
		ParentIssue wireIssue `json:"parent_issue"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode sub_issues payload: %w", err)
	}
	// Both sub_issue_* and parent_issue_* actions describe the same parent->child
	// edge; the payload always carries both objects, so direction is explicit.
	parent, child := p.ParentIssue.Number, p.SubIssue.Number
	switch p.Action {
	case "sub_issue_added", "parent_issue_added":
		return []Mutation{AddHierarchy(parent, child)}, nil
	case "sub_issue_removed", "parent_issue_removed":
		return []Mutation{RemoveHierarchy(parent, child)}, nil
	default:
		return nil, nil
	}
}

func translateDependencies(body []byte) ([]Mutation, error) {
	var p struct {
		Action        string    `json:"action"`
		BlockedIssue  wireIssue `json:"blocked_issue"`
		BlockingIssue wireIssue `json:"blocking_issue"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decode issue_dependencies payload: %w", err)
	}
	// The payload always carries both blocked_issue and blocking_issue, so the
	// edge is fully specified regardless of which side (blocked_by_*/blocking_*)
	// fired. blocking_issue blocks blocked_issue.
	blocked, blocker := p.BlockedIssue.Number, p.BlockingIssue.Number
	switch p.Action {
	case "blocked_by_added", "blocking_added":
		return []Mutation{AddDependency(blocked, blocker)}, nil
	case "blocked_by_removed", "blocking_removed":
		return []Mutation{RemoveDependency(blocked, blocker)}, nil
	default:
		return nil, nil
	}
}
