import type { EpicStatus } from "../gen/triage/v1/triage_pb";

// The renderable graph distilled from a Board. It is a DAG: epics at the top,
// hierarchy edges down through ancestry to ready leaves, and dashed "blocks"
// edges from a stalled epic to the prerequisite holding it up. Nodes shared
// across epics are deduped into one vertex (the high-value convergence case).

export type NodeKind = "epic" | "ready" | "blocker" | "ancestry";

export interface GraphNode {
  id: string; // issue number as string (ELK node id)
  number: number;
  title: string;
  url: string;
  kind: NodeKind;
  status?: EpicStatus; // epics only
  leverage?: number;
  highLeverage?: boolean;
  multiEpic?: boolean;
}

export type EdgeKind = "hierarchy" | "blocks";

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  kind: EdgeKind;
}

export interface BoardGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}
