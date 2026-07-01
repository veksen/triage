import { useEffect, useMemo, useRef, useState } from "react";
import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkEdgeSection, ElkExtendedEdge, ElkNode, ElkPoint } from "elkjs/lib/elk-api";
import { line, curveLinear } from "d3-shape";
import { select } from "d3-selection";
import { zoom, zoomIdentity, type ZoomBehavior } from "d3-zoom";
import "d3-transition";
import { EpicStatus } from "../gen/triage/v1/triage_pb";
import type { Board } from "../gen/triage/v1/triage_pb";
import { buildBoardGraph } from "../graph/buildBoardGraph";
import { toElk } from "../graph/layout";
import type { GraphNode } from "../graph/model";

interface LaidNode extends GraphNode {
  x: number;
  y: number;
  w: number;
  h: number;
}
interface LaidEdge {
  id: string;
  kind: string;
  d: string;
  label?: string;
  lx: number;
  ly: number;
}
interface Laid {
  nodes: LaidNode[];
  edges: LaidEdge[];
  width: number;
  height: number;
}

const elk = new ELK();
// Crisp linear segments through ELK's orthogonal bend points — right-angle wiring
// like a dependency map, not smoothed curves.
const edgePath = line<ElkPoint>()
  .x((p) => p.x)
  .y((p) => p.y)
  .curve(curveLinear);

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
  [EpicStatus.EMPTY]: "no work",
};

// GraphView renders the board as a dependency map: epics anchor the top, ready
// work hangs off "contains" edges, and a stalled epic wires to the prerequisite
// holding it up via a labelled "blocked by" edge — what blocks what, and why
// (leverage) it matters. ELK lays out the DAG; d3 draws the wiring and drives
// pan/zoom.
export function GraphView({ board }: { board: Board | undefined }) {
  const graph = useMemo(() => buildBoardGraph(board), [board]);
  const [laid, setLaid] = useState<Laid | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const viewportRef = useRef<SVGGElement>(null);
  const zoomRef = useRef<ZoomBehavior<SVGSVGElement, unknown> | null>(null);

  // Layout (async). Guard against a stale board resolving after a newer one.
  useEffect(() => {
    if (graph.nodes.length === 0) {
      setLaid({ nodes: [], edges: [], width: 0, height: 0 });
      return;
    }
    let cancelled = false;
    elk
      .layout(toElk(graph) as ElkNode)
      .then((res) => {
        if (cancelled) return;
        const meta = new Map(graph.nodes.map((n) => [n.id, n]));
        const nodes: LaidNode[] = (res.children ?? []).map((c) => ({
          ...meta.get(c.id ?? "")!,
          x: c.x ?? 0,
          y: c.y ?? 0,
          w: c.width ?? 216,
          h: c.height ?? 58,
        }));
        const kindOf = new Map(graph.edges.map((e) => [e.id, e.kind]));
        const edges: LaidEdge[] = (res.edges ?? []).map((e: ElkExtendedEdge) => {
          const s: ElkEdgeSection | undefined = e.sections?.[0];
          const pts = s ? [s.startPoint, ...(s.bendPoints ?? []), s.endPoint] : [];
          const kind = kindOf.get(e.id) ?? "hierarchy";
          const mid = polyMidpoint(pts);
          return {
            id: e.id,
            kind,
            d: edgePath(pts) ?? "",
            label: kind === "blocks" ? "blocked by" : undefined,
            lx: mid.x,
            ly: mid.y,
          };
        });
        setLaid({ nodes, edges, width: res.width ?? 0, height: res.height ?? 0 });
      })
      .catch((err) => {
        if (!cancelled) console.error("graph layout failed", err);
      });
    return () => {
      cancelled = true;
    };
  }, [graph]);

  // Pan/zoom, set up once.
  useEffect(() => {
    if (!svgRef.current) return;
    const svg = select(svgRef.current);
    const z = zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 2.5])
      .on("zoom", (e) => {
        if (viewportRef.current) select(viewportRef.current).attr("transform", e.transform.toString());
      });
    zoomRef.current = z;
    svg.call(z);
    return () => {
      svg.on(".zoom", null);
    };
  }, []);

  const fitTo = (l: Laid | null) => {
    if (!l || !svgRef.current || !zoomRef.current || l.nodes.length === 0) return;
    const svg = svgRef.current;
    const cw = svg.clientWidth || 900;
    const ch = svg.clientHeight || 600;
    const pad = 56;
    const scale = Math.max(0.2, Math.min(1.4, Math.min((cw - 2 * pad) / l.width, (ch - 2 * pad) / l.height)));
    const tx = (cw - l.width * scale) / 2;
    const ty = pad;
    select(svg).transition().duration(300).call(zoomRef.current.transform, zoomIdentity.translate(tx, ty).scale(scale));
  };

  // Fit whenever a new layout arrives.
  useEffect(() => {
    fitTo(laid);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [laid]);

  const zoomBy = (k: number) => {
    if (svgRef.current && zoomRef.current) {
      select(svgRef.current).transition().duration(180).call(zoomRef.current.scaleBy, k);
    }
  };

  if (graph.nodes.length === 0) {
    return (
      <p className="status muted">
        No active epics to graph. Open an epic (or remove its <code>parked</code> label) to populate it.
      </p>
    );
  }

  return (
    <div className="graph-wrap">
      <svg ref={svgRef} className="graph-svg" role="img" aria-label="dependency graph">
        <defs>
          <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L10,5 L0,10 z" className="arrow arrow--hierarchy" />
          </marker>
          <marker id="arrow-blocks" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
            <path d="M0,0 L10,5 L0,10 z" className="arrow arrow--blocks" />
          </marker>
        </defs>
        <g ref={viewportRef}>
          {laid?.edges.map((e) => (
            <path
              key={e.id}
              d={e.d}
              className={`gedge gedge--${e.kind}`}
              markerEnd={e.kind === "blocks" ? "url(#arrow-blocks)" : "url(#arrow)"}
            />
          ))}
          {laid?.edges
            .filter((e) => e.label)
            .map((e) => (
              <g key={`${e.id}-label`} className="edge-label" transform={`translate(${e.lx},${e.ly})`}>
                <rect x={-38} y={-9} width={76} height={18} rx={9} />
                <text y={4}>{e.label}</text>
              </g>
            ))}
          {laid?.nodes.map((n) => (
            <GraphNodeView key={n.id} node={n} />
          ))}
        </g>
      </svg>

      <div className="graph-toolbar" role="toolbar" aria-label="graph controls">
        <button type="button" title="Zoom out" onClick={() => zoomBy(1 / 1.25)}>
          −
        </button>
        <button type="button" title="Fit to view" onClick={() => fitTo(laid)}>
          Fit
        </button>
        <button type="button" title="Zoom in" onClick={() => zoomBy(1.25)}>
          +
        </button>
      </div>

      <ul className="legend">
        <li>
          <span className="swatch swatch--epic-active" /> epic (ready)
        </li>
        <li>
          <span className="swatch swatch--epic-stalled" /> epic (stalled)
        </li>
        <li>
          <span className="swatch swatch--ready" /> ready leaf
        </li>
        <li>
          <span className="swatch swatch--blocked" /> blocked task
        </li>
        <li>
          <span className="swatch swatch--blocker" /> blocker
        </li>
        <li>
          <span className="dash" /> blocked by
        </li>
      </ul>
    </div>
  );
}

function GraphNodeView({ node }: { node: LaidNode }) {
  const epicSlug = node.kind === "epic" && node.status !== undefined ? STATUS_SLUG[node.status] : "";
  const cls = `gnode gnode--${node.kind}${epicSlug ? ` gnode--epic-${epicSlug}` : ""}`;

  const chip =
    node.kind === "epic" && node.status !== undefined
      ? STATUS_LABEL[node.status]
      : node.leverage
        ? `unblocks ${node.leverage}`
        : "";
  const chipCls =
    node.kind === "epic" && node.status !== undefined ? `gnode-chip gnode-chip--${epicSlug}` : "gnode-chip";

  const maxChars = 25;
  const title = node.title.length > maxChars ? node.title.slice(0, maxChars - 1) + "…" : node.title;

  return (
    <g transform={`translate(${node.x},${node.y})`}>
      <a href={node.url || undefined} target="_blank" rel="noreferrer">
        <rect className={cls} width={node.w} height={node.h} rx={9} />
        <rect className={`gnode-accent gnode-accent--${node.kind}${epicSlug ? `-${epicSlug}` : ""}`} width={4} height={node.h} rx={2} />
        <text className="gnode-num" x={14} y={22}>
          #{node.number}
        </text>
        {chip && (
          <text className={chipCls} x={node.w - 12} y={22} textAnchor="end">
            {chip}
          </text>
        )}
        <text className="gnode-title" x={14} y={42}>
          {title}
        </text>
      </a>
    </g>
  );
}

// polyMidpoint returns the point at half the arc length of a polyline — a stable
// spot to anchor an edge label.
function polyMidpoint(pts: ElkPoint[]): ElkPoint {
  if (pts.length === 0) return { x: 0, y: 0 };
  if (pts.length === 1) return pts[0];
  let total = 0;
  const seg: number[] = [];
  for (let i = 1; i < pts.length; i++) {
    const len = Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
    seg.push(len);
    total += len;
  }
  let acc = 0;
  const target = total / 2;
  for (let i = 1; i < pts.length; i++) {
    const len = seg[i - 1];
    if (acc + len >= target) {
      const t = len ? (target - acc) / len : 0;
      return { x: pts[i - 1].x + (pts[i].x - pts[i - 1].x) * t, y: pts[i - 1].y + (pts[i].y - pts[i - 1].y) * t };
    }
    acc += len;
  }
  return pts[pts.length - 1];
}
