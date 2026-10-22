import { apiClient } from "./client";
import { postEmpty, postJSON, putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type TerminalForward = components["schemas"]["TerminalForward"];
export type TerminalSession = components["schemas"]["TerminalSession"];
export type TerminalSessionList = components["schemas"]["TerminalSessionList"];
export type OpenTerminalSessionRequest = components["schemas"]["OpenTerminalSessionRequest"];
export type OpenTerminalSessionResponse = components["schemas"]["OpenTerminalSessionResponse"];
export type StartTerminalForwardRequest = components["schemas"]["StartTerminalForwardRequest"];
export type TerminalStreamTicket = components["schemas"]["TerminalStreamTicket"];

export type TerminalSessionsApi = {
  // streamPath is the WebSocket path a stream ticket from this API is redeemed at.
  streamPath: string;
  terminalSessions(): Promise<TerminalSessionList>;
  openTerminalSession(
    request: OpenTerminalSessionRequest,
  ): Promise<OpenTerminalSessionResponse>;
  terminalStreamTicket(id: string, cursor?: number): Promise<TerminalStreamTicket>;
  reconnectTerminalSession(id: string): Promise<TerminalSessionList>;
  stopTerminalReconnect(id: string): Promise<TerminalSessionList>;
  startTerminalForward(
    id: string,
    request: StartTerminalForwardRequest,
  ): Promise<TerminalSessionList>;
  stopTerminalForward(
    id: string,
    forwardId: string,
  ): Promise<TerminalSessionList>;
  setTerminalSessionTitle(
    id: string,
    title: string | null,
  ): Promise<TerminalSessionList>;
  closeTerminalSession(id: string): Promise<TerminalSessionList>;
  moveTerminalSession(id: string, workspaceId: string): Promise<TerminalSessionList>;
};

function validateTerminalSessionList(value: unknown): TerminalSessionList {
  return validateOpenAPISchema<TerminalSessionList>("TerminalSessionList", value);
}

function validateOpenTerminalSession(
  value: unknown,
): OpenTerminalSessionResponse {
  return validateOpenAPISchema<OpenTerminalSessionResponse>("OpenTerminalSessionResponse", value);
}

function validateStreamTicket(value: unknown): TerminalStreamTicket {
  return validateOpenAPISchema<TerminalStreamTicket>("TerminalStreamTicket", value);
}

// The terminal sessions of one engine: opening, streaming, reconnecting,
// forwarding, renaming and closing them. `base` is that engine's terminal API:
// this machine's /api/v1/terminal, or a run-target engine's
// /api/v1/engines/{alias}/terminal that this machine's engine forwards.
export function createTerminalSessionsApi(base: string, streamPath: string): TerminalSessionsApi {
  return {
    streamPath,
    async terminalSessions() {
      return validateTerminalSessionList(
        await apiClient.read(`${base}/sessions`),
      );
    },
    async openTerminalSession(request) {
      return validateOpenTerminalSession(
        await postJSON<unknown>(`${base}/sessions`, request),
      );
    },
    async terminalStreamTicket(id, cursor) {
      const suffix = cursor === undefined ? "" : `?cursor=${encodeURIComponent(String(cursor))}`;
      return validateStreamTicket(
        await postEmpty<unknown>(
          `${base}/sessions/${encodeURIComponent(id)}/stream${suffix}`,
        ),
      );
    },
    async reconnectTerminalSession(id) {
      return validateTerminalSessionList(
        await postEmpty<unknown>(
          `${base}/sessions/${encodeURIComponent(id)}/reconnect`,
        ),
      );
    },
    async stopTerminalReconnect(id) {
      return validateTerminalSessionList(
        await postEmpty<unknown>(
          `${base}/sessions/${encodeURIComponent(id)}/reconnect/stop`,
        ),
      );
    },
    async startTerminalForward(id, request) {
      return validateTerminalSessionList(
        await postJSON<unknown>(
          `${base}/sessions/${encodeURIComponent(id)}/forwards`,
          request,
        ),
      );
    },
    async stopTerminalForward(id, forwardId) {
      return validateTerminalSessionList(
        await apiClient.mutate<unknown>(
          `${base}/sessions/${encodeURIComponent(id)}/forwards/${encodeURIComponent(forwardId)}`,
          {
            method: "DELETE",
          },
        ),
      );
    },
    async setTerminalSessionTitle(id, title) {
      return validateTerminalSessionList(
        await putJSON<unknown>(`${base}/sessions/${encodeURIComponent(id)}/title`, { title }),
      );
    },
    async moveTerminalSession(id, workspaceId) {
      return validateTerminalSessionList(
        await putJSON<unknown>(`${base}/sessions/${encodeURIComponent(id)}/workspace`, { workspaceId }),
      );
    },
    async closeTerminalSession(id) {
      return validateTerminalSessionList(
        await apiClient.mutate<unknown>(
          `${base}/sessions/${encodeURIComponent(id)}`,
          {
            method: "DELETE",
          },
        ),
      );
    },
  };
}

// This machine's own terminal sessions.
export const terminalSessionsApi = createTerminalSessionsApi("/api/v1/terminal", "/terminal/stream");
