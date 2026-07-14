import { test, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { BoardSchema, EpicStatus, EpicViewSchema, IssueRefSchema, NodeSchema } from "../gen/triage/v1/triage_pb";
import { SwimlaneView } from "./SwimlaneView";

afterEach(cleanup);

const ref = (n: number, t: string) => create(IssueRefSchema, { number: BigInt(n), title: t, open: true });

test("renders an epic lane with full, untruncated task titles", () => {
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, {
        epic: ref(1, "epic: unify analyzer output across PR comment, alert, MCP"),
        status: EpicStatus.ACTIVE,
        ready: [create(NodeSchema, { issue: ref(10, "[output-parity] feat(api): consistent verdict schema"), leverage: 2 })],
      }),
    ],
  });
  render(<SwimlaneView board={board} />);

  expect(screen.getByText("epic: unify analyzer output across PR comment, alert, MCP")).toBeTruthy();
  expect(screen.getByText("[output-parity] feat(api): consistent verdict schema")).toBeTruthy();
  expect(screen.getByText("unblocks 2")).toBeTruthy();
});

test("empty epics show a no-work lane and sink below epics with work", () => {
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, { epic: ref(3, "Empty epic"), status: EpicStatus.EMPTY }),
      create(EpicViewSchema, {
        epic: ref(1, "Active epic"),
        status: EpicStatus.ACTIVE,
        ready: [create(NodeSchema, { issue: ref(10, "A task") })],
      }),
    ],
  });
  render(<SwimlaneView board={board} />);

  expect(screen.getByText("No open work.")).toBeTruthy();
  const titles = screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
  expect(titles).toEqual(["Active epic", "Empty epic"]); // active first
});

test("a stalled epic lists its blockers", () => {
  const board = create(BoardSchema, {
    epics: [
      create(EpicViewSchema, {
        epic: ref(2, "Stalled epic"),
        status: EpicStatus.STALLED,
        blockers: [create(NodeSchema, { issue: ref(99, "External prerequisite") })],
      }),
    ],
  });
  render(<SwimlaneView board={board} />);

  expect(screen.getByText("stalled")).toBeTruthy();
  expect(screen.getByText("External prerequisite")).toBeTruthy();
});
