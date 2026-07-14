import { useState } from "react";
import type { EpicView } from "../gen/triage/v1/triage_pb";

// EpicFilter is the "what do I want to track" control. It works off a set of
// hidden epic numbers (empty = show all), so epics arriving later over the stream
// default to visible without any resync.
export function EpicFilter({
  epics,
  hidden,
  onChange,
}: {
  epics: EpicView[];
  hidden: Set<number>;
  onChange: (next: Set<number>) => void;
}) {
  const [open, setOpen] = useState(false);
  if (epics.length === 0) return null;

  const shown = epics.length - hidden.size;
  const label = hidden.size === 0 ? "all epics" : `${shown} of ${epics.length}`;

  const setVisible = (n: number, visible: boolean) => {
    const next = new Set(hidden);
    if (visible) next.delete(n);
    else next.add(n);
    onChange(next);
  };
  const only = (n: number) => {
    const next = new Set<number>();
    for (const e of epics) {
      const m = Number(e.epic?.number);
      if (m !== n) next.add(m);
    }
    onChange(next);
  };

  return (
    <div className="filter">
      <button className="filter-btn" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        Tracking: <strong>{label}</strong>
        <span className="caret" aria-hidden>
          ▾
        </span>
      </button>

      {open && (
        <>
          <div className="filter-backdrop" onClick={() => setOpen(false)} />
          <div className="filter-menu" role="menu">
            <div className="filter-head">
              <span>Track epics</span>
              <button className="linkish" onClick={() => onChange(new Set())} disabled={hidden.size === 0}>
                Show all
              </button>
            </div>
            <ul>
              {epics.map((e) => {
                const n = Number(e.epic?.number);
                const visible = !hidden.has(n);
                return (
                  <li key={n}>
                    <label>
                      <input type="checkbox" checked={visible} onChange={(ev) => setVisible(n, ev.target.checked)} />
                      <span className="fnum">#{n}</span>
                      <span className="ftitle">{e.epic?.title || "(untitled epic)"}</span>
                    </label>
                    <button className="only" onClick={() => only(n)} title="Track only this epic">
                      only
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        </>
      )}
    </div>
  );
}
