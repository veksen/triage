// Command triage-server runs the Connect server over an in-memory engine.
//
// Until the GitHub sync layer lands, the engine starts from an empty graph and
// the server serves a (correctly) empty board — it streams real updates the
// moment something feeds the engine through Apply.
//
// The Connect protocol streams over HTTP/1.1, so no h2c/HTTP-2 cleartext setup
// is needed for the TS Connect client. Add h2c here if cleartext gRPC clients
// are ever required.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/veksen/triage/gen/triage/v1/triagev1connect"
	"github.com/veksen/triage/internal/engine"
	"github.com/veksen/triage/internal/graph"
	"github.com/veksen/triage/internal/server"
)

func main() {
	addr := envOr("TRIAGE_ADDR", ":8080")
	repoURL := os.Getenv("TRIAGE_REPO_URL") // e.g. https://github.com/veksen/triage

	eng := engine.New(graph.NewGraph(), engine.Config{RepoURL: repoURL})

	mux := http.NewServeMux()
	path, handler := triagev1connect.NewTriageServiceHandler(server.New(eng))
	mux.Handle(path, handler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("triage-server listening on %s (empty board until sync lands)", addr)
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
