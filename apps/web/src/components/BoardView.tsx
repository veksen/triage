import { useMemo, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { BoardSchema } from "../gen/triage/v1/triage_pb";
import { useBoard } from "../useBoard";
import { EpicFilter } from "./EpicFilter";
import { GraphView } from "./GraphView";
import { ListView } from "./ListView";

type View = "graph" | "list";

// BoardView owns the single board subscription, the view toggle, and the epic
// filter. Filtering is client-side (the board is small and read-only): it hides
// the deselected epics, and buildBoardGraph already drops any dependency edge
// whose endpoints aren't rendered, so the graph prunes cleanly.
export function BoardView() {
  const { data: board, isPending, error } = useBoard();
  const [view, setView] = useState<View>("graph");
  const [hidden, setHidden] = useState<Set<number>>(new Set());

  const allEpics = board?.epics ?? [];

  // Memoized so filtering doesn't rebuild the board (and re-run the ELK layout)
  // on every render. Unfiltered → reuse the original board's identity.
  const shownBoard = useMemo(() => {
    if (!board || hidden.size === 0) return board;
    const epics = board.epics.filter((e) => !hidden.has(Number(e.epic?.number)));
    return create(BoardSchema, { epics, dependencies: board.dependencies });
  }, [board, hidden]);

  const shownCount = shownBoard?.epics.length ?? 0;
  const countLabel = isPending
    ? "connecting…"
    : hidden.size > 0
      ? `${shownCount} of ${allEpics.length} epics`
      : `${allEpics.length} active epic${allEpics.length === 1 ? "" : "s"}`;

  return (
    <main className="app">
      <header className="topbar">
        <h1>triage</h1>
        <span className="muted">{countLabel}</span>
        <div className="spacer" />
        <EpicFilter epics={allEpics} hidden={hidden} onChange={setHidden} />
        <div className="toggle" role="tablist" aria-label="view">
          <button role="tab" aria-selected={view === "graph"} className={view === "graph" ? "on" : ""} onClick={() => setView("graph")}>
            graph
          </button>
          <button role="tab" aria-selected={view === "list"} className={view === "list" ? "on" : ""} onClick={() => setView("list")}>
            list
          </button>
        </div>
      </header>

      {error ? (
        <p className="status error">Failed to load the board: {String(error)}</p>
      ) : isPending ? (
        <p className="status muted">Loading board…</p>
      ) : view === "graph" ? (
        <GraphView board={shownBoard} />
      ) : (
        <ListView board={shownBoard} />
      )}
    </main>
  );
}
