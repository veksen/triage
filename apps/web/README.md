# @triage/web

The frontend for the derived dependency board. Vite + React + TypeScript, with a
Connect-ES client generated from the same `proto/` the Go server is built from —
the proto is the contract, so client and server never drift.

## Develop

```bash
npm install
npm run generate     # proto -> src/gen (re-run when proto/ changes)
npm run dev          # Vite dev server on :5173, proxies the API to :8080
```

Run the backend alongside it. For data without a real GitHub backfill, seed it:

```bash
# from the repo root
TRIAGE_DEV_SEED=1 go run ./cmd/triage-server
```

## Scripts

| script | what |
|---|---|
| `npm run generate` | regenerate the TS client from `../../proto` (`buf generate`) |
| `npm run dev` | Vite dev server (API proxied to `localhost:8080`) |
| `npm run build` | typecheck + production build to `dist/` |
| `npm run test` | Vitest component tests |
| `npm run typecheck` | `tsc --noEmit` |

`VITE_API_BASE_URL` overrides the API origin (default same-origin) for split
hosting.

## Data flow

`useBoard` fetches the initial snapshot with TanStack Query (`GetBoard`), then
subscribes to `StreamBoard` and **replaces** the cached board on every push —
the server sends full snapshots, so the client never reconciles deltas.

## Views

Two views over the same cached `Board` (one `StreamBoard` subscription, toggled
in the top bar):

- **graph** (default) — the dependency DAG. **elkjs** computes a layered
  top-down layout (raw d3 can't lay out a DAG); **d3** renders it — `d3-shape`
  for the curved edges, `d3-zoom` for pan/zoom. Epics colour by status, ready
  leaves hang off solid hierarchy edges, and a stalled epic's blocker hangs off
  a dashed "blocks" edge. `src/graph/buildBoardGraph.ts` is the pure,
  unit-tested Board→DAG transform.
- **list** — the structured column-per-epic detail view, for scanning exact
  ready/blocker items.

Both views expose the one write — **park** an epic (`SetEpicState` active=false)
from its node/column. The server mutates the projection and pushes a fresh board
over the stream, so the parked epic disappears through the normal update path (no
optimistic cache edit). Note: the board only shows *active* epics, so unparking
needs a "parked epics" affordance the contract doesn't expose yet — a future RPC.

## Next (visual iteration)

Possible refinements: collapse/expand deep ancestry, fit-to-selection, edge
labels for leverage, and code-splitting the elkjs bundle (it dominates the JS
payload). The data contract is fixed; these are all presentation changes.
