package sync

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/veksen/triage/internal/engine"
	"github.com/veksen/triage/internal/github"
	"github.com/veksen/triage/internal/graph"
)

// maxWebhookBody caps the payload we read; GitHub deliveries are well under this.
const maxWebhookBody = 8 << 20 // 8 MiB

// WebhookHandler verifies and applies GitHub webhook deliveries. Each valid
// delivery is translated to graph mutations and applied through the engine in a
// single Apply, so one delivery yields exactly one recompute and broadcast.
type WebhookHandler struct {
	engine *engine.Engine
	mapper github.Mapper
	secret []byte
}

// NewWebhookHandler returns a handler that applies deliveries to eng. secret is
// the webhook secret used to verify the X-Hub-Signature-256 header; an empty
// secret rejects every delivery (fail closed).
func NewWebhookHandler(eng *engine.Engine, m github.Mapper, secret []byte) *WebhookHandler {
	return &WebhookHandler{engine: eng, mapper: m, secret: secret}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	// Verify before parsing: never act on an unauthenticated payload.
	if !ValidSignature(h.secret, r.Header.Get("X-Hub-Signature-256"), body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	muts, err := h.mapper.Translate(event, body)
	if err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	if len(muts) > 0 {
		h.engine.Apply(func(g *graph.Graph) {
			for _, m := range muts {
				m(g)
			}
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// ValidSignature reports whether header is a valid HMAC-SHA256 signature of body
// under secret, in GitHub's "sha256=<hex>" form. It fails closed: an empty
// secret, a missing/misformatted header, or a mismatch all return false. The
// comparison is constant-time.
func ValidSignature(secret []byte, header string, body []byte) bool {
	const prefix = "sha256="
	if len(secret) == 0 || !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(want, mac.Sum(nil))
}

// Sign returns the "sha256=<hex>" signature for body under secret. It exists so
// callers (and tests) can produce the header GitHub would send.
func Sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
