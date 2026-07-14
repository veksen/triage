import { EpicStatus } from "../gen/triage/v1/triage_pb";
import type { Board, EpicView, Node as PbNode } from "../gen/triage/v1/triage_pb";

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

// SwimlaneView is the readable board: one lane per epic, lanes stacked top to
// bottom, and each epic's tasks laid out horizontally across its lane. Full
// titles, no truncation — you scan down for epics and across for their work.
export function SwimlaneView({ board }: { board: Board | undefined }) {
  const epics = board?.epics ?? [];
  if (epics.length === 0) {
    return (
      <p className="status muted">
        No active epics. Open an epic (or remove its <code>parked</code> label) to populate the board.
      </p>
    );
  }
  // Epics with work first; empty ones sink to the bottom.
  const ordered = [...epics].sort((a, b) => rank(a.status) - rank(b.status));
  return (
    <div className="lanes">
      {ordered.map((ev) => (
        <Lane key={ev.epic ? String(ev.epic.number) : "?"} epic={ev} />
      ))}
    </div>
  );
}

function rank(s: EpicStatus): number {
  return s === EpicStatus.EMPTY || s === EpicStatus.UNSPECIFIED ? 1 : 0;
}

function Lane({ epic }: { epic: EpicView }) {
  const slug = STATUS_SLUG[epic.status];
  const stalled = epic.status === EpicStatus.STALLED;
  const tasks = stalled ? epic.blockers : epic.ready;

  return (
    <section className={`lane lane--${slug}`}>
      <header className="lane-head">
        <div className="lane-title-row">
          <a className="ref" href={epic.epic?.url || undefined} target="_blank" rel="noreferrer">
            #{epic.epic ? String(epic.epic.number) : "?"}
          </a>
          <span className={`badge badge--${slug}`}>{STATUS_LABEL[epic.status]}</span>
        </div>
        <h2 className="lane-title">{epic.epic?.title || "(untitled epic)"}</h2>
        {tasks.length > 0 && (
          <span className="lane-count">
            {tasks.length} {stalled ? (tasks.length === 1 ? "blocker" : "blockers") : "ready"}
          </span>
        )}
      </header>

      <div className="lane-tasks">
        {tasks.length === 0 ? (
          <p className="lane-empty">{epic.status === EpicStatus.EMPTY ? "No open work." : "Nothing actionable."}</p>
        ) : (
          tasks.map((n, i) => <TaskCard key={n.issue ? String(n.issue.number) : i} node={n} kind={stalled ? "blocker" : "ready"} />)
        )}
      </div>
    </section>
  );
}

function TaskCard({ node, kind }: { node: PbNode; kind: "ready" | "blocker" }) {
  const ref = node.issue;
  return (
    <a className={`task task--${kind}`} href={ref?.url || undefined} target="_blank" rel="noreferrer">
      <div className="task-meta">
        <span className="task-num">#{ref ? String(ref.number) : "?"}</span>
        {node.leverage > 0 && <span className="task-lev">unblocks {node.leverage}</span>}
      </div>
      <div className="task-title">{ref?.title || "(untitled)"}</div>
    </a>
  );
}
