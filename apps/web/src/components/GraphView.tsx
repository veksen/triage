import { useEffect, useMemo, useRef, useState } from "react";
import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkEdgeSection, ElkExtendedEdge, ElkNode } from "elkjs/lib/elk-api";
import { line, curveBasis } from "d3-shape";
import { select } from "d3-selection";
import { zoom, zoomIdentity, type ZoomBehavior } from "d3-zoom";
import "d3-transition"; // enables selection.transition() used in the fit effect
import { EpicStatus } from "../gen/triage/v1/triage_pb";
import type { Board } from "../gen/triage/v1/triage_pb";
import { buildBoardGraph } from "../graph/buildBoardGraph";
import { toElk } from "../graph/layout";
import type { GraphNode } from "../graph/model";
import { useSetEpicState } from "../useSetEpicState";

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
}
interface Laid {
  nodes: LaidNode[];
  edges: LaidEdge[];
  width: number;
  height: number;
}

const elk = new ELK();
const edgePath = line<{ x: number; y: number }>()
  .x((p) => p.x)
  .y((p) => p.y)
  .curve(curveBasis);

const STATUS_SLUG: Record<EpicStatus, string> = {
  [EpicStatus.UNSPECIFIED]: "unknown",
  [EpicStatus.ACTIVE]: "active",
  [EpicStatus.STALLED]: "stalled",
  [EpicStatus.EMPTY]: "empty",
};

// GraphView lays the board DAG out with ELK, then renders it as SVG. React owns
// the DOM (nodes/edges as JSX); d3 does the maths it is good at — d3-shape for
// smooth edge curves and d3-zoom for pan/zoom — the "React for DOM, d3 for the
// numbers" split, with ELK supplying the layout d3 alone can't.
export function GraphView({ board }: { board: Board | undefined }) {
  const graph = useMemo(() => buildBoardGraph(board), [board]);
  const [laid, setLaid] = useState<Laid | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const viewportRef = useRef<SVGGElement>(null);
  const zoomRef = useRef<ZoomBehavior<SVGSVGElement, unknown> | null>(null);
  const park = useSetEpicState();
  const parkingNumber = park.isPending ? park.variables?.epicNumber : undefined;

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
          w: c.width ?? 132,
          h: c.height ?? 46,
        }));
        const kindOf = new Map(graph.edges.map((e) => [e.id, e.kind]));
        const edges: LaidEdge[] = (res.edges ?? []).map((e: ElkExtendedEdge) => {
          const s: ElkEdgeSection | undefined = e.sections?.[0];
          const pts = s ? [s.startPoint, ...(s.bendPoints ?? []), s.endPoint] : [];
          return { id: e.id, kind: kindOf.get(e.id) ?? "hierarchy", d: edgePath(pts) ?? "" };
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

  // Fit the graph to the viewport whenever a new layout arrives.
  useEffect(() => {
    if (!laid || !svgRef.current || !zoomRef.current || laid.nodes.length === 0) return;
    const svg = svgRef.current;
    const cw = svg.clientWidth || 900;
    const ch = svg.clientHeight || 600;
    const pad = 48;
    const scale = Math.max(0.2, Math.min(1.4, Math.min((cw - 2 * pad) / laid.width, (ch - 2 * pad) / laid.height)));
    const tx = (cw - laid.width * scale) / 2;
    const ty = pad;
    select(svg)
      .transition()
      .duration(300)
      .call(zoomRef.current.transform, zoomIdentity.translate(tx, ty).scale(scale));
  }, [laid]);

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
          <marker id="arrow-blocks" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
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
          {laid?.nodes.map((n) => (
            <GraphNodeView
              key={n.id}
              node={n}
              onPark={n.kind === "epic" ? (epicNumber) => park.mutate({ epicNumber, active: false }) : undefined}
              parking={n.number === parkingNumber}
            />
          ))}
        </g>
      </svg>
      <Legend />
    </div>
  );
}

function GraphNodeView({
  node,
  onPark,
  parking,
}: {
  node: LaidNode;
  onPark?: (epicNumber: number) => void;
  parking?: boolean;
}) {
  const epicSlug = node.kind === "epic" && node.status !== undefined ? STATUS_SLUG[node.status] : "";
  const cls = `gnode gnode--${node.kind}${epicSlug ? ` gnode--epic-${epicSlug}` : ""}`;
  // Leave room for the park button on epic nodes so the title never overlaps it.
  const rightInset = onPark ? 52 : 22;
  const maxChars = Math.max(4, Math.floor((node.w - rightInset) / 7));
  const title = node.title.length > maxChars ? node.title.slice(0, maxChars - 1) + "…" : node.title;

  return (
    <g transform={`translate(${node.x},${node.y})`}>
      <a href={node.url || undefined} target="_blank" rel="noreferrer">
        <rect className={cls} width={node.w} height={node.h} rx={9} />
        <text className="gnode-num" x={11} y={19}>
          #{node.number}
          {node.leverage ? `  ·  unblocks ${node.leverage}` : ""}
        </text>
        <text className="gnode-title" x={11} y={35}>
          {title}
        </text>
        {!onPark && node.highLeverage && <circle className="dot dot--high" cx={node.w - 13} cy={13} r={4} />}
        {!onPark && node.multiEpic && (
          <circle className="dot dot--multi" cx={node.w - 13} cy={node.highLeverage ? 27 : 13} r={4} />
        )}
      </a>

      {onPark && (
        // Sibling of the <a>, painted on top, so the click parks rather than
        // following the issue link.
        <g
          className={`park-node ${parking ? "is-parking" : ""}`}
          transform={`translate(${node.w - 46},8)`}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            if (!parking) onPark(node.number);
          }}
        >
          <rect className="park-node-bg" width={38} height={16} rx={5} />
          <text className="park-node-text" x={19} y={12}>
            {parking ? "…" : "park"}
          </text>
        </g>
      )}
    </g>
  );
}

function Legend() {
  return (
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
        <span className="swatch swatch--blocker" /> blocker
      </li>
      <li>
        <span className="dash" /> blocks
      </li>
    </ul>
  );
}
