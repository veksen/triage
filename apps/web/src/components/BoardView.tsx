import { useState } from "react";
import { useBoard } from "../useBoard";
import { GraphView } from "./GraphView";
import { ListView } from "./ListView";

type View = "graph" | "list";

// BoardView owns the single board subscription and switches between the graph
// overview ("it's a graph, not a board") and the column list detail. Both views
// read the same cached Board, so there is only ever one StreamBoard connection.
export function BoardView() {
  const { data: board, isPending, error } = useBoard();
  const [view, setView] = useState<View>("graph");
  const epics = board?.epics ?? [];

  return (
    <main className="app">
      <header className="topbar">
        <h1>triage</h1>
        <span className="muted">
          {isPending ? "connecting…" : `${epics.length} active epic${epics.length === 1 ? "" : "s"}`}
        </span>
        <div className="spacer" />
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
        <GraphView board={board} />
      ) : (
        <ListView board={board} />
      )}
    </main>
  );
}
