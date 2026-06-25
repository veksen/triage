# triage

A **derived** dependency dashboard. Instead of a hand-groomed project board that
decays the moment people stop grooming it, `triage` computes the board from data
already maintained in GitHub (issues, sub-issue hierarchy, native `blocked by` /
`blocking` dependencies) and answers two questions at a glance:

1. **What's next?** — what you can actually pick up right now.
2. **Why are we stuck?** — when nothing is ready, the specific work holding an
   epic up.

GitHub is the only source of truth; the dashboard reads and reflects, it never
holds authoritative state.

## Core principle

Readiness alone is not the filter — hundreds of issues are "ready" and most are
irrelevant. **Relevance to an active epic is the filter.** The rendered graph is
the subgraph induced by the currently-active epics; an issue appears only if it
lies on a path toward an active epic.

## Active epics — the one human lever

Board membership is a **pure function of epic state**, so there is no manual
board to fall out of sync:

> An epic drives the board when it is **open and not parked**.

"Free play by default": opening an epic puts it on the board; the rare,
deliberate action is _parking_ it (the `parked` label) to take it off. Closing
or parking an epic removes its whole ladder automatically — the graph
self-cleans. The polarity lives in a single configurable predicate
(`DefaultActive`), so flipping to opt-in later is a config change, not a rewrite.

## Status — first deliverable

The pure node-selection algorithm + its tests (`internal/graph`). No GitHub, no
transport, no database yet — the algorithm is deliberately decoupled so it is
fully testable against fixture graphs.

```
go test ./...
```

### The algorithm (`internal/graph`)

`Compute(graph, options)` runs, per active epic:

1. **Scope** — the epic's open hierarchy descendants (its _ladder_) plus the
   transitive open blockers of that ladder.
2. **Ready frontier** — ladder issues that are open with no open blocker. These
   are the "go do this now" leaves.
3. **Stall detection** — when a ladder is fully blocked, the epic is _stalled_;
   it surfaces the nearest **actionable** open blockers instead (the ready roots
   of the blocking chain). These may be out of the epic's scope — that
   cross-epic blocker is the high-value signal, not a bug.
4. **Rank** — ready leaves are ordered by leverage (how many in-scope issues
   each transitively unblocks).
5. **Render set** — each leaf carries 1–3 levels of hierarchy ancestry for
   context; nodes shared across epics are flagged multi-epic / high-leverage.

#### One spec disambiguation

The design doc's `scope(e)` is _ladder ∪ blocker-closure_. We keep those subsets
distinct: the **ready frontier is drawn from the ladder only**, while the blocker
closure feeds **stall detection**. Treating every ready in-scope issue as the
frontier would make stall detection dead code (a finite DAG's blocker closure
always has ready roots). Splitting them is what makes "ready leaves = what's
next; the blocking node when nothing's ready = why we're stuck" hold. See the
comment block atop `internal/graph/select.go`.

## Planned stack

- **Backend:** Go. GitHub sync (REST + webhooks) maintains the cached projection
  and runs the selection above.
- **Contract + transport:** Connect-RPC — one proto, typed Go server + TS client,
  with **server-streaming** to push the render set on change (no separate
  WebSocket/SSE layer).
- **Frontend:** TypeScript + Vite + React, a graph layout/render layer (d3 +
  elkjs / React Flow), TanStack Query/Router, Vitest, oxlint/oxfmt.

The visual form is deliberately left to iterate; the data contract (the render
set above) is what's fixed.

## Layout

```
proto/triage/v1/triage.proto   # the cross-language contract (Connect-RPC)
gen/triage/v1/                 # generated Go + Connect stubs (buf generate)
internal/graph/                # the pure node-selection algorithm + tests
internal/api/                  # adapter: graph.Result -> wire Board
internal/engine/               # owns the projection; recompute + streaming hub
internal/server/               # Connect handlers over the engine
internal/github/               # pure GitHub payload -> graph mutation mapping
internal/sync/                 # REST backfill + HMAC-verified webhook handler
cmd/triage-server/             # runnable entrypoint (backfill + webhook + serve)
apps/web/                      # Vite + React client; TS client generated from proto
```

## Codegen

The wire types are generated from `proto/` with [buf](https://buf.build) and the
Connect/protobuf Go plugins. The generated code is committed so the module
builds without the toolchain installed.

```
make tools      # install pinned buf + protoc-gen-go + protoc-gen-connect-go
make generate   # buf lint + buf generate
make test       # go test ./...
```

The service (`triage.v1.TriageService`) exposes `GetBoard` (unary snapshot),
`StreamBoard` (server-streaming, full snapshots on change), and `SetEpicState`
(the one write: drive or park an epic).

## Roadmap

- [x] Core node-selection algorithm + tests
- [x] Connect-RPC schema for the render set + "mark epic active/parked"
- [x] Adapter from the algorithm result to the wire board
- [x] Projection engine + streaming hub (`internal/engine`)
- [x] Connect server: `GetBoard` / `StreamBoard` / `SetEpicState` + entrypoint
- [x] GitHub sync layer: REST backfill + HMAC-verified webhook updates
- [x] Frontend tracer slice: generated TS client + live board view (`apps/web`)
- [ ] Frontend graph visualization (elkjs/React Flow over the same board)
