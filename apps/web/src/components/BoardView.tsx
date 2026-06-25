import { useBoard } from "../useBoard";
import { EpicColumn } from "./EpicColumn";

// BoardView is the whole screen: the render set is one column per active epic.
// It is a structured view of the fixed Board contract — the graph-layout
// visualisation (elkjs / React Flow) is the deliberate next visual iteration;
// the data plumbing it will draw from is what's settled here.
export function BoardView() {
  const { data: board, isPending, error } = useBoard();
  const epics = board?.epics ?? [];

  return (
    <main className="app">
      <header className="topbar">
        <h1>triage</h1>
        <span className="muted">
          {isPending ? "connecting…" : `${epics.length} active epic${epics.length === 1 ? "" : "s"}`}
        </span>
      </header>

      {error ? (
        <p className="status error">Failed to load the board: {String(error)}</p>
      ) : isPending ? (
        <p className="status muted">Loading board…</p>
      ) : epics.length === 0 ? (
        <p className="status muted">
          No active epics. Open an epic (or remove its <code>parked</code> label) to populate the board.
        </p>
      ) : (
        <div className="board">
          {epics.map((e) => (
            <EpicColumn key={e.epic ? String(e.epic.number) : "?"} epic={e} />
          ))}
        </div>
      )}
    </main>
  );
}
