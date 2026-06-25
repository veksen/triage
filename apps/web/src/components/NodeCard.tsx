import type { Node } from "../gen/triage/v1/triage_pb";

// NodeCard renders one rendered issue: its ref, leverage, the hierarchy "why"
// trail, and the high-value flags (high-leverage, multi-epic). blocker styles a
// node that is surfaced because it is holding a stalled epic up.
export function NodeCard({ node, blocker }: { node: Node; blocker: boolean }) {
  const ref = node.issue;
  return (
    <li className={`node ${blocker ? "node--blocker" : "node--ready"}`}>
      <div className="node-head">
        <a className="ref" href={ref?.url || undefined} target="_blank" rel="noreferrer">
          #{ref ? String(ref.number) : "?"}
        </a>
        <span className="node-title">{ref?.title || "(untitled)"}</span>
      </div>

      <div className="flags">
        {node.leverage > 0 && (
          <span className="flag" title="open in-scope issues this transitively unblocks">
            unblocks {node.leverage}
          </span>
        )}
        {node.highLeverage && (
          <span className="flag flag--high" title="leverage at or above the threshold">
            high-leverage
          </span>
        )}
        {node.multiEpic && (
          <span className="flag flag--multi" title={`serves ${node.servesEpics.length} active epics`}>
            multi-epic
          </span>
        )}
      </div>

      {node.ancestry.length > 0 && (
        <div className="ancestry" title="hierarchy path toward the epic">
          {node.ancestry.map((a) => `#${String(a.number)}`).join(" ‹ ")}
        </div>
      )}
    </li>
  );
}
