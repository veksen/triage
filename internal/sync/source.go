// Package sync keeps the engine's projection in step with GitHub. It is the
// network/HTTP shell around the pure internal/github mapping: a REST backfill
// builds the initial graph on startup, and a webhook handler applies live
// mutations as GitHub events arrive. All translation logic lives in
// internal/github; this package only moves bytes and verifies signatures.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/veksen/triage/internal/github"
)

// Source is the read side of GitHub the backfill needs. Client implements it
// over REST; tests provide a fake. Keeping it an interface is what makes
// Backfill unit-testable without a network.
type Source interface {
	ListOpenIssues(ctx context.Context) ([]github.Issue, error)
	ListSubIssues(ctx context.Context, parent int) ([]int, error)
	ListBlockedBy(ctx context.Context, issue int) ([]int, error)
}

// apiVersion pins the GitHub REST API version that exposes sub-issues and native
// issue dependencies.
const apiVersion = "2026-03-10"

const defaultBaseURL = "https://api.github.com"

// Client is a minimal GitHub REST client covering exactly the three reads the
// backfill performs. It intentionally avoids a heavyweight SDK: the endpoints
// are new and few, and the module keeps its dependency surface small.
type Client struct {
	http    *http.Client
	baseURL string
	owner   string
	repo    string
	token   string
}

// NewClient returns a Client for owner/repo authenticated with token. baseURL
// defaults to the public API; pass an override (e.g. an httptest server) for
// tests or GitHub Enterprise.
func NewClient(owner, repo, token string, opts ...Option) *Client {
	c := &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: defaultBaseURL,
		owner:   owner,
		repo:    repo,
		token:   token,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (no trailing slash).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient overrides the underlying http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// apiIssue is the slice of GitHub's REST issue object we read. We take number
// (per-repo), never id (global). pull_request distinguishes PRs, which the
// issues endpoint also returns and we must drop.
type apiIssue struct {
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	State       string          `json:"state"`
	Labels      []apiLabel      `json:"labels"`
	Type        *apiNamed       `json:"type"`
	Repository  *apiRepo        `json:"repository"`
	PullRequest json.RawMessage `json:"pull_request"`
}

type apiLabel struct {
	Name string `json:"name"`
}

type apiNamed struct {
	Name string `json:"name"`
}

type apiRepo struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func (a apiIssue) toIssue() github.Issue {
	labels := make([]string, 0, len(a.Labels))
	for _, l := range a.Labels {
		labels = append(labels, l.Name)
	}
	iss := github.Issue{Number: a.Number, Title: a.Title, State: a.State, Labels: labels}
	if a.Type != nil {
		iss.TypeName = a.Type.Name
	}
	return iss
}

// ListOpenIssues returns every open issue (pull requests filtered out) across
// all pages.
func (c *Client) ListOpenIssues(ctx context.Context) ([]github.Issue, error) {
	var out []github.Issue
	err := c.paginate(ctx, fmt.Sprintf("/repos/%s/%s/issues", c.owner, c.repo),
		url.Values{"state": {"open"}},
		func(page []apiIssue) {
			for _, a := range page {
				if a.PullRequest != nil {
					continue // the issues endpoint also returns PRs
				}
				out = append(out, a.toIssue())
			}
		})
	return out, err
}

// ListSubIssues returns the numbers of parent's direct sub-issues.
func (c *Client) ListSubIssues(ctx context.Context, parent int) ([]int, error) {
	var out []int
	err := c.paginate(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d/sub_issues", c.owner, c.repo, parent),
		nil,
		func(page []apiIssue) {
			for _, a := range page {
				out = append(out, a.Number)
			}
		})
	return out, err
}

// ListBlockedBy returns the numbers of the issues that block the given issue,
// restricted to this repository — a cross-repo blocker's number would collide
// with a local one in the single-repo projection, so it is dropped (a known
// limitation; cross-repo dependencies are future work).
func (c *Client) ListBlockedBy(ctx context.Context, issue int) ([]int, error) {
	var out []int
	err := c.paginate(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d/dependencies/blocked_by", c.owner, c.repo, issue),
		nil,
		func(page []apiIssue) {
			for _, a := range page {
				if a.Repository != nil && !c.sameRepo(a.Repository) {
					continue
				}
				out = append(out, a.Number)
			}
		})
	return out, err
}

func (c *Client) sameRepo(r *apiRepo) bool {
	return strings.EqualFold(r.Owner.Login, c.owner) && strings.EqualFold(r.Name, c.repo)
}

// paginate walks every page of a list endpoint, invoking collect on each. It
// stops when a page is shorter than the requested size — no Link-header parsing
// needed for these small result sets.
func (c *Client) paginate(ctx context.Context, path string, query url.Values, collect func([]apiIssue)) error {
	const perPage = 100
	if query == nil {
		query = url.Values{}
	}
	query.Set("per_page", strconv.Itoa(perPage))
	for page := 1; ; page++ {
		query.Set("page", strconv.Itoa(page))
		var batch []apiIssue
		if err := c.get(ctx, path+"?"+query.Encode(), &batch); err != nil {
			return err
		}
		collect(batch)
		if len(batch) < perPage {
			return nil
		}
	}
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
