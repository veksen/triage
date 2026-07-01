import { useMutation } from "@tanstack/react-query";
import { client } from "./client";

// useSetEpicState drives (active=true) or parks (active=false) an epic. It does
// not touch the query cache: the server mutates the projection and pushes a new
// board over StreamBoard, so the UI updates through the same self-healing path
// as every other change. isPending + variables let a caller show which epic is
// in flight and disable its control.
export function useSetEpicState() {
  return useMutation({
    mutationFn: ({ epicNumber, active }: { epicNumber: number; active: boolean }) =>
      client.setEpicState({ epicNumber: BigInt(epicNumber), active }),
  });
}
