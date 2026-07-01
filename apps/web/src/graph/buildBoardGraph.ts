import { EpicStatus } from "../gen/triage/v1/triage_pb";
import type { Board, IssueRef, Node as PbNode } from "../gen/triage/v1/triage_pb";
import type { BoardGraph, EdgeKind, GraphEdge, GraphNode, NodeKind } from "./model";

// Higher-precedence kinds win when the same issue surfaces in several roles
// (e.g. a node that is one epic's leaf and another's ancestry stays the leaf).
const KIND_RANK: Record<NodeKind, number> = { epic: 3, ready: 2, blocker: 2, ancestry: 1 };

// buildBoardGraph turns the wire Board into a DAG for layout. Pure: no ELK, no
// d3, no DOM — so the edge/dedup logic is unit-testable against fixtures.
export function buildBoardGraph(board: Board | undefined): BoardGraph {
  const nodes = new Map<number, GraphNode>();
  const edges = new Map<string, GraphEdge>();

  const upsert = (ref: IssueRef | undefined, kind: NodeKind, extra: Partial<GraphNode> = {}) => {
    if (!ref) return;
    const n = Number(ref.number);
    const existing = nodes.get(n);
    if (existing && KIND_RANK[existing.kind] >= KIND_RANK[kind]) {
      mergeFlags(existing, extra); // keep the stronger kind, but don't lose flags
      return;
    }
    nodes.set(n, { id: String(n), number: n, title: ref.title, url: ref.url, kind, ...extra });
  };

  const addEdge = (source: number, target: number, kind: EdgeKind) => {
    if (source === target) return; // ancestry that reaches the epic would self-loop
    const id = `${source}->${target}`;
    if (!edges.has(id)) edges.set(id, { id, source: String(source), target: String(target), kind });
  };

  // addReady wires epic -> (ancestry chain) -> ready leaf with hierarchy edges.
  // ancestry is nearest-first ([parent, grandparent, ...]), so we walk it linking
  // each level to its child and finally attach the topmost known ancestor (or the
  // leaf itself) to the epic. This chain is the "why does this matter" context.
  const addReady = (epicNumber: number, leaf: PbNode) => {
    upsert(leaf.issue, "ready", {
      leverage: leaf.leverage,
      highLeverage: leaf.highLeverage,
      multiEpic: leaf.multiEpic,
    });
    let child = Number(leaf.issue?.number ?? 0);
    for (const a of leaf.ancestry) {
      upsert(a, "ancestry");
      addEdge(Number(a.number), child, "hierarchy");
      child = Number(a.number);
    }
    addEdge(epicNumber, child, "hierarchy");
  };

  // addBlocker links the stalled epic to the prerequisite holding it up with a
  // "blocks" edge (rendered "blocked by", pointing at the culprit). We don't
  // chain a blocker's ancestry: a blocker is typically out of the epic's scope,
  // so its ladder context is noise here — the blocking relationship is the point.
  const addBlocker = (epicNumber: number, b: PbNode) => {
    upsert(b.issue, "blocker", {
      leverage: b.leverage,
      highLeverage: b.highLeverage,
      multiEpic: b.multiEpic,
    });
    addEdge(epicNumber, Number(b.issue?.number ?? 0), "blocks");
  };

  for (const ev of board?.epics ?? []) {
    upsert(ev.epic, "epic", { status: ev.status });
    const epicNumber = Number(ev.epic?.number ?? 0);
    if (ev.status === EpicStatus.STALLED) {
      for (const b of ev.blockers) addBlocker(epicNumber, b);
    } else {
      for (const r of ev.ready) addReady(epicNumber, r);
    }
  }

  return { nodes: [...nodes.values()], edges: [...edges.values()] };
}

function mergeFlags(node: GraphNode, extra: Partial<GraphNode>) {
  if (extra.highLeverage) node.highLeverage = true;
  if (extra.multiEpic) node.multiEpic = true;
  if (typeof extra.leverage === "number" && extra.leverage > (node.leverage ?? 0)) {
    node.leverage = extra.leverage;
  }
}
