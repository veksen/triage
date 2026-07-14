import type { ElkNode } from "elkjs/lib/elk-api";
import type { BoardGraph } from "./model";

// Uniform card size — a dependency map reads as a grid of comparable cards, so
// every node is the same box and the title truncates to fit.
export const NODE_WIDTH = 216;
export const NODE_HEIGHT = 58;

// toElk converts the board DAG into ELK's input graph. Layered + ORTHOGONAL edge
// routing gives the right-angle "what blocks what" wiring of a dependency map —
// the DAG layout raw d3 can't do, which is why a layout engine feeds d3 here.
//
// Each epic tree is usually a disconnected component; aspectRatio tells ELK how
// to pack them so they fill the viewport (side-by-side on a wide screen) rather
// than stacking into a tall, narrow column.
export function toElk(graph: BoardGraph, aspectRatio = 1.6): ElkNode {
  return {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.layered.spacing.nodeNodeBetweenLayers": "72",
      "elk.spacing.nodeNode": "20",
      "elk.layered.spacing.edgeNodeBetweenLayers": "32",
      "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
      "elk.separateConnectedComponents": "true",
      "elk.spacing.componentComponent": "56",
      "elk.aspectRatio": String(aspectRatio),
    },
    children: graph.nodes.map((n) => ({ id: n.id, width: NODE_WIDTH, height: NODE_HEIGHT })),
    edges: graph.edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  };
}
