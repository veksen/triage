import type { ElkNode } from "elkjs/lib/elk-api";
import type { BoardGraph } from "./model";

export const NODE_HEIGHT = 46;

// nodeWidth sizes a node to its "#num title" label. Rough char-width estimate —
// good enough for layout; the SVG text is truncated to fit at render time.
export function nodeWidth(title: string, number: number): number {
  const label = `#${number}  ${title}`;
  return Math.min(300, Math.max(132, label.length * 7.2 + 32));
}

// toElk converts the board DAG into ELK's input graph. We use the layered
// algorithm top-down — exactly the DAG layout raw d3 can't do, which is why a
// layout engine feeds the d3 rendering rather than d3 placing nodes itself.
export function toElk(graph: BoardGraph): ElkNode {
  return {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.layered.spacing.nodeNodeBetweenLayers": "64",
      "elk.spacing.nodeNode": "28",
      "elk.layered.spacing.edgeNodeBetweenLayers": "24",
      "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
    },
    children: graph.nodes.map((n) => ({
      id: n.id,
      width: nodeWidth(n.title, n.number),
      height: NODE_HEIGHT,
    })),
    edges: graph.edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  };
}
