import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { TriageService } from "./gen/triage/v1/triage_pb";

// Same-origin by default (dev proxies the service path to the Go server; a
// production deploy serves both behind one origin). Override for split hosting.
const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "/";

export const transport = createConnectTransport({ baseUrl });

// One typed client built from the generated service descriptor — the proto is
// the contract, so these calls stay in lockstep with the Go server.
export const client = createClient(TriageService, transport);
