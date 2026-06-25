import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "./client";
import type { Board } from "./gen/triage/v1/triage_pb";

const KEY = ["board"] as const;

// useBoard fetches the initial snapshot with TanStack Query, then subscribes to
// StreamBoard and replaces the cached board on every push. Because the server
// sends full snapshots (not deltas), "replace" is all the client ever does —
// no reconciliation, and a reconnect re-primes from the first streamed message.
export function useBoard() {
  const qc = useQueryClient();

  const query = useQuery({
    queryKey: KEY,
    queryFn: () => client.getBoard({}).then((r) => r.board),
  });

  useEffect(() => {
    const ac = new AbortController();
    (async () => {
      try {
        for await (const res of client.streamBoard({}, { signal: ac.signal })) {
          qc.setQueryData<Board | undefined>(KEY, res.board);
        }
      } catch (err) {
        if (!ac.signal.aborted) {
          // A dropped stream isn't fatal — the last snapshot stays on screen
          // and the query can refetch; surface it for visibility.
          console.error("board stream error", err);
        }
      }
    })();
    return () => ac.abort();
  }, [qc]);

  return query;
}
