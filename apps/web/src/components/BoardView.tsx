import { useMemo, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { BoardSchema } from "../gen/triage/v1/triage_pb";
import { useBoard } from "../useBoard";
import { EpicFilter } from "./EpicFilter";
import { SwimlaneView } from "./SwimlaneView";
import { GraphView } from "./GraphView";

type View = "board" | "graph";

// BoardView owns the single board subscription, the view toggle, and the epic
// filter. The swimlane board is the default (readable: one lane per epic, tasks
// across); the graph is opt-in for tracing blocking chains. Filtering is
// client-side over the cached board.
export function BoardView() {
  const { data: board, isPending, error } = useBoard();
  const [view, setView] = useState<View>("board");
  const [hidden, setHidden] = useState<Set<number>>(new Set());

  const allEpics = board?.epics ?? [];

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
          <button role="tab" aria-selected={view === "board"} className={view === "board" ? "on" : ""} onClick={() => setView("board")}>
            board
          </button>
          <button role="tab" aria-selected={view === "graph"} className={view === "graph" ? "on" : ""} onClick={() => setView("graph")}>
            graph
          </button>
        </div>
      </header>

      {error ? (
        <p className="status error">Failed to load the board: {String(error)}</p>
      ) : isPending ? (
        <p className="status muted">Loading board…</p>
      ) : view === "board" ? (
        <SwimlaneView
          board={shownBoard}
          onUntrack={(n) =>
            setHidden((prev) => {
              const next = new Set(prev);
              next.add(n);
              return next;
            })
          }
        />
      ) : (
        <GraphView board={shownBoard} />
      )}
    </main>
  );
}
