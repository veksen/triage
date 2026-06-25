import { test, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { EpicColumn } from "./EpicColumn";
import {
  EpicStatus,
  EpicViewSchema,
  NodeSchema,
  IssueRefSchema,
} from "../gen/triage/v1/triage_pb";

afterEach(cleanup);

function ref(number: number, title: string) {
  return create(IssueRefSchema, { number: BigInt(number), title, open: true });
}

test("an ACTIVE epic shows its ready leaves with flags", () => {
  const epic = create(EpicViewSchema, {
    epic: ref(1, "Ship dashboard"),
    status: EpicStatus.ACTIVE,
    ready: [
      create(NodeSchema, {
        issue: ref(10, "Wire the API"),
        leverage: 4,
        highLeverage: true,
        multiEpic: true,
        servesEpics: [1n, 2n],
        ancestry: [ref(5, "Story")],
      }),
    ],
  });

  render(<EpicColumn epic={epic} />);

  expect(screen.getByText("Ship dashboard")).toBeTruthy();
  expect(screen.getByText("ready")).toBeTruthy();
  expect(screen.getByText("Wire the API")).toBeTruthy();
  expect(screen.getByText("high-leverage")).toBeTruthy();
  expect(screen.getByText("multi-epic")).toBeTruthy();
  expect(screen.getByText("#5")).toBeTruthy(); // ancestry trail
});

test("a STALLED epic shows blockers and the why-stuck hint", () => {
  const epic = create(EpicViewSchema, {
    epic: ref(1, "Ship dashboard"),
    status: EpicStatus.STALLED,
    blockers: [create(NodeSchema, { issue: ref(99, "External prerequisite") })],
  });

  render(<EpicColumn epic={epic} />);

  expect(screen.getByText("stalled")).toBeTruthy();
  expect(screen.getByText(/holding it up/i)).toBeTruthy();
  expect(screen.getByText("External prerequisite")).toBeTruthy();
});

test("an EMPTY epic renders the no-open-work state", () => {
  const epic = create(EpicViewSchema, {
    epic: ref(1, "Ship dashboard"),
    status: EpicStatus.EMPTY,
  });

  render(<EpicColumn epic={epic} />);

  expect(screen.getByText("no open work")).toBeTruthy();
  expect(screen.getByText(/no open ladder work/i)).toBeTruthy();
});
