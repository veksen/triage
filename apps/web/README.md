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

## Next (visual iteration)

The current screen is a structured column-per-epic view of the fixed `Board`
contract. The planned next step is a real graph layout (elkjs / dagre feeding
React Flow / `@xyflow`) — "it's a graph, not a board". The data plumbing it will
draw from is already settled here; only the visual form changes.
