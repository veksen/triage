import { EpicStatus } from "../gen/triage/v1/triage_pb";
import type { EpicView } from "../gen/triage/v1/triage_pb";
import { NodeCard } from "./NodeCard";

const STATUS_SLUG: Record<EpicStatus, string> = {
  [EpicStatus.UNSPECIFIED]: "unknown",
  [EpicStatus.ACTIVE]: "active",
  [EpicStatus.STALLED]: "stalled",
  [EpicStatus.EMPTY]: "empty",
};

const STATUS_LABEL: Record<EpicStatus, string> = {
  [EpicStatus.UNSPECIFIED]: "unknown",
  [EpicStatus.ACTIVE]: "ready",
  [EpicStatus.STALLED]: "stalled",
  [EpicStatus.EMPTY]: "no open work",
};

interface EpicColumnProps {
  epic: EpicView;
  // onPark, when provided, renders a park control that removes the epic from the
  // board (SetEpicState active=false). parking marks it in flight.
  onPark?: (epicNumber: number) => void;
  parking?: boolean;
}

// EpicColumn renders one active epic: its ready frontier when ACTIVE, or the
// actionable blockers holding it up when STALLED ("what's next" vs "why stuck").
export function EpicColumn({ epic, onPark, parking }: EpicColumnProps) {
  const stalled = epic.status === EpicStatus.STALLED;
  const slug = STATUS_SLUG[epic.status];
  const nodes = stalled ? epic.blockers : epic.ready;
  const epicNumber = epic.epic ? Number(epic.epic.number) : undefined;

  return (
    <section className={`epic epic--${slug}`}>
      <header className="epic-head">
        <a className="ref" href={epic.epic?.url || undefined} target="_blank" rel="noreferrer">
          #{epic.epic ? String(epic.epic.number) : "?"}
        </a>
        <h2 className="epic-title">{epic.epic?.title || "(untitled epic)"}</h2>
        <span className={`badge badge--${slug}`}>{STATUS_LABEL[epic.status]}</span>
        {onPark && epicNumber !== undefined && (
          <button
            type="button"
            className="park-btn"
            disabled={parking}
            title="Park this epic — removes its ladder from the board"
            onClick={() => onPark(epicNumber)}
          >
            {parking ? "parking…" : "park"}
          </button>
        )}
      </header>

      {stalled && (
        <p className="hint">Blocked — these open prerequisites are holding it up:</p>
      )}

      {nodes.length === 0 ? (
        <p className="muted small">
          {epic.status === EpicStatus.EMPTY ? "No open ladder work." : "Nothing actionable right now."}
        </p>
      ) : (
        <ul className="nodes">
          {nodes.map((n, i) => (
            <NodeCard key={n.issue ? String(n.issue.number) : i} node={n} blocker={stalled} />
          ))}
        </ul>
      )}
    </section>
  );
}
