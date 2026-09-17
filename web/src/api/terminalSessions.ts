import { apiClient } from "./client";
import { jsonHeaders, postEmpty, postJSON } from "./guards";
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
  terminalSessions(): Promise<TerminalSessionList>;
  openTerminalSession(
    request: OpenTerminalSessionRequest,
  ): Promise<OpenTerminalSessionResponse>;
  terminalStreamTicket(id: string, cursor?: number): Promise<TerminalStreamTicket>;
  reconnectTerminalSession(id: string): Promise<TerminalSessionList>;
  startTerminalForward(
    id: string,
    request: StartTerminalForwardRequest,
  ): Promise<TerminalSessionList>;
  stopTerminalForward(
    id: string,
    forwardId: string,
  ): Promise<TerminalSessionList>;
  renameTerminalSession(
    id: string,
    title: string | null,
  ): Promise<TerminalSessionList>;
  closeTerminalSession(id: string): Promise<TerminalSessionList>;
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

// The engine's live terminal sessions: opening, streaming, reconnecting,
// forwarding, renaming and closing them.
export const terminalSessionsApi: TerminalSessionsApi = {
  async terminalSessions() {
    return validateTerminalSessionList(
      await apiClient.read("/api/v1/terminal/sessions"),
    );
  },
  async openTerminalSession(request) {
    return validateOpenTerminalSession(
      await postJSON<unknown>("/api/v1/terminal/sessions", request),
    );
  },
  async terminalStreamTicket(id, cursor) {
    const suffix = cursor === undefined ? "" : `?cursor=${encodeURIComponent(String(cursor))}`;
    return validateStreamTicket(
      await postEmpty<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/stream${suffix}`,
      ),
    );
  },
  async reconnectTerminalSession(id) {
    return validateTerminalSessionList(
      await postEmpty<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/reconnect`,
      ),
    );
  },
  async startTerminalForward(id, request) {
    return validateTerminalSessionList(
      await postJSON<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/forwards`,
        request,
      ),
    );
  },
  async stopTerminalForward(id, forwardId) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/forwards/${encodeURIComponent(forwardId)}`,
        {
          method: "DELETE",
        },
      ),
    );
  },
  async renameTerminalSession(id, title) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/title`,
        {
          method: "PUT",
          headers: { ...jsonHeaders },
          body: JSON.stringify({ title }),
        },
      ),
    );
  },
  async closeTerminalSession(id) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}`,
        {
          method: "DELETE",
        },
      ),
    );
  },
};
