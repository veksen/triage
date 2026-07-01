import type { Board } from "../gen/triage/v1/triage_pb";
import { EpicColumn } from "./EpicColumn";

// ListView is the structured column-per-epic view — good for scanning the exact
// ready/blocker items. The graph view is the overview; this is the detail.
// Read-only, like the whole board.
export function ListView({ board }: { board: Board | undefined }) {
  const epics = board?.epics ?? [];
  if (epics.length === 0) {
    return (
      <p className="status muted">
        No active epics. Open an epic (or remove its <code>parked</code> label) to populate the board.
      </p>
    );
  }
  return (
    <div className="board">
      {epics.map((e) => (
        <EpicColumn key={e.epic ? String(e.epic.number) : "?"} epic={e} />
      ))}
    </div>
  );
}
