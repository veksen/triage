import type { ElkNode } from "elkjs/lib/elk-api";
import type { BoardGraph } from "./model";

// Uniform card size — a dependency map reads as a grid of comparable cards, so
// every node is the same box and the title truncates to fit.
export const NODE_WIDTH = 216;
export const NODE_HEIGHT = 58;

// toElk converts the board DAG into ELK's input graph. Layered + ORTHOGONAL edge
// routing gives the right-angle "what blocks what" wiring of a dependency map —
// the DAG layout raw d3 can't do, which is why a layout engine feeds d3 here.
export function toElk(graph: BoardGraph): ElkNode {
  return {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.layered.spacing.nodeNodeBetweenLayers": "88",
      "elk.spacing.nodeNode": "40",
      "elk.layered.spacing.edgeNodeBetweenLayers": "32",
      "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
    },
    children: graph.nodes.map((n) => ({ id: n.id, width: NODE_WIDTH, height: NODE_HEIGHT })),
    edges: graph.edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  };
}
