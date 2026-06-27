import { test, expect } from "vitest";
import { create } from "@bufbuild/protobuf";
import {
  BoardSchema,
  EpicStatus,
  EpicViewSchema,
  IssueRefSchema,
  NodeSchema,
} from "../gen/triage/v1/triage_pb";
import { buildBoardGraph } from "./buildBoardGraph";

const ref = (number: number, title = `issue ${number}`) =>
  create(IssueRefSchema, { number: BigInt(number), title, open: true });

function findEdge(edges: { source: string; target: string; kind: string }[], s: number, t: number) {
  return edges.find((e) => e.source === String(s) && e.target === String(t));
}

test("an ACTIVE epic links epic -> ancestry -> ready leaf with hierarchy edges", () => {
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, {
        epic: ref(1, "Ship dashboard"),
        status: EpicStatus.ACTIVE,
        ready: [
          create(NodeSchema, {
            issue: ref(10, "Wire the API"),
            ancestry: [ref(5, "Story")], // 5 is 10's parent toward epic 1
          }),
        ],
      }),
    ],
  });

  const g = buildBoardGraph(board);

  expect(g.nodes.map((n) => n.number).sort((a, b) => a - b)).toEqual([1, 5, 10]);
  expect(g.nodes.find((n) => n.number === 1)?.kind).toBe("epic");
  expect(g.nodes.find((n) => n.number === 5)?.kind).toBe("ancestry");
  expect(g.nodes.find((n) => n.number === 10)?.kind).toBe("ready");
  // chain: 5 -> 10 (parent), and epic 1 -> 5 (top of chain)
  expect(findEdge(g.edges, 5, 10)?.kind).toBe("hierarchy");
  expect(findEdge(g.edges, 1, 5)?.kind).toBe("hierarchy");
});

test("a STALLED epic links epic -> blocker with a dashed blocks edge", () => {
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, {
        epic: ref(1),
        status: EpicStatus.STALLED,
        blockers: [create(NodeSchema, { issue: ref(99, "External prerequisite") })],
      }),
    ],
  });

  const g = buildBoardGraph(board);
  expect(g.nodes.find((n) => n.number === 99)?.kind).toBe("blocker");
  expect(findEdge(g.edges, 1, 99)?.kind).toBe("blocks");
});

test("a node serving two epics is deduped into one vertex with both edges", () => {
  const shared = () => create(NodeSchema, { issue: ref(10, "Shared"), multiEpic: true, leverage: 4 });
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, { epic: ref(1), status: EpicStatus.ACTIVE, ready: [shared()] }),
      create(EpicViewSchema, { epic: ref(2), status: EpicStatus.ACTIVE, ready: [shared()] }),
    ],
  });

  const g = buildBoardGraph(board);
  expect(g.nodes.filter((n) => n.number === 10)).toHaveLength(1); // deduped
  const node10 = g.nodes.find((n) => n.number === 10);
  expect(node10?.multiEpic).toBe(true);
  // both epics point at the shared node — the convergence the graph should show
  expect(findEdge(g.edges, 1, 10)).toBeTruthy();
  expect(findEdge(g.edges, 2, 10)).toBeTruthy();
});

test("ancestry that reaches the epic does not create a self-loop", () => {
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, {
        epic: ref(1),
        status: EpicStatus.ACTIVE,
        ready: [create(NodeSchema, { issue: ref(10), ancestry: [ref(1)] })], // parent IS the epic
      }),
    ],
  });

  const g = buildBoardGraph(board);
  expect(g.edges.every((e) => e.source !== e.target)).toBe(true);
  expect(findEdge(g.edges, 1, 10)?.kind).toBe("hierarchy");
});

test("empty board yields an empty graph", () => {
  expect(buildBoardGraph(undefined)).toEqual({ nodes: [], edges: [] });
  expect(buildBoardGraph(create(BoardSchema, {}))).toEqual({ nodes: [], edges: [] });
});
