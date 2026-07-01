import type { Board } from "../gen/triage/v1/triage_pb";
import { useSetEpicState } from "../useSetEpicState";
import { EpicColumn } from "./EpicColumn";

// ListView is the structured column-per-epic view — good for scanning the exact
// ready/blocker items. The graph view is the overview; this is the detail.
export function ListView({ board }: { board: Board | undefined }) {
  const park = useSetEpicState();
  const epics = board?.epics ?? [];
  const parkingNumber = park.isPending ? park.variables?.epicNumber : undefined;

  if (epics.length === 0) {
    return (
      <p className="status muted">
        No active epics. Open an epic (or remove its <code>parked</code> label) to populate the board.
      </p>
    );
  }
  return (
    <div className="board">
      {epics.map((e) => {
        const n = e.epic ? Number(e.epic.number) : undefined;
        return (
          <EpicColumn
            key={n ?? "?"}
            epic={e}
            onPark={(epicNumber) => park.mutate({ epicNumber, active: false })}
            parking={n !== undefined && n === parkingNumber}
          />
        );
      })}
    </div>
  );
}
