// Command triage-server runs the Connect server over an in-memory engine,
// optionally backfilled from GitHub and kept live by a webhook endpoint.
//
// With no GitHub configuration it starts from an empty graph and serves a
// (correctly) empty board — handy for local frontend work. Set the env vars
// below to sync against a real repo:
//
//	TRIAGE_GITHUB_OWNER   repo owner (enables backfill when set with REPO)
//	TRIAGE_GITHUB_REPO    repo name
//	TRIAGE_GITHUB_TOKEN   token for the REST backfill
//	TRIAGE_WEBHOOK_SECRET HMAC secret; mounts POST /webhook when set
//	TRIAGE_EPIC_LABEL     label that marks an epic (default "epic")
//	TRIAGE_REPO_URL       repo web base for issue links
//	TRIAGE_ADDR           listen address (default ":8080")
//
// The Connect protocol streams over HTTP/1.1, so no h2c/HTTP-2 cleartext setup
// is needed for the TS client.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/veksen/triage/gen/triage/v1/triagev1connect"
	"github.com/veksen/triage/internal/engine"
	"github.com/veksen/triage/internal/github"
	"github.com/veksen/triage/internal/graph"
	"github.com/veksen/triage/internal/server"
	triagesync "github.com/veksen/triage/internal/sync"
)

func main() {
	addr := envOr("TRIAGE_ADDR", ":8080")
	repoURL := os.Getenv("TRIAGE_REPO_URL")
	owner := os.Getenv("TRIAGE_GITHUB_OWNER")
	repo := os.Getenv("TRIAGE_GITHUB_REPO")

	mapper := github.NewMapper()
	if l := os.Getenv("TRIAGE_EPIC_LABEL"); l != "" {
		mapper.EpicLabel = l
	}

	g := graph.NewGraph()
	switch {
	case owner != "" && repo != "":
		client := triagesync.NewClient(owner, repo, os.Getenv("TRIAGE_GITHUB_TOKEN"))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		log.Printf("backfilling board from %s/%s …", owner, repo)
		built, err := triagesync.Backfill(ctx, client, mapper)
		cancel()
		if err != nil {
			log.Fatalf("backfill from %s/%s failed: %v", owner, repo, err)
		}
		g = built
	case os.Getenv("TRIAGE_DEV_SEED") != "":
		g = devSeed()
		log.Print("loaded dev seed graph (TRIAGE_DEV_SEED set)")
	}

	eng := engine.New(g, engine.Config{RepoURL: repoURL})
	if owner != "" && repo != "" {
		log.Printf("backfilled %s/%s: %d active epics on the board", owner, repo, len(eng.Board().GetEpics()))
	}

	mux := http.NewServeMux()
	path, handler := triagev1connect.NewTriageServiceHandler(server.New(eng))
	mux.Handle(path, handler)

	if secret := os.Getenv("TRIAGE_WEBHOOK_SECRET"); secret != "" {
		mux.Handle("/webhook", triagesync.NewWebhookHandler(eng, mapper, []byte(secret)))
		log.Print("webhook endpoint mounted at POST /webhook")
	} else {
		log.Print("TRIAGE_WEBHOOK_SECRET unset — webhook endpoint disabled")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("triage-server listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
