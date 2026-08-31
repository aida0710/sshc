import { describe, expect, it } from "vitest";
import {
  validateAPIRequest,
  validateAPIResponse,
  validateOpenAPISchema,
} from "./validators.generated";

describe("generated OpenAPI runtime validators", () => {
  it("validates declared requests and exact response statuses", () => {
    expect(() => validateAPIRequest("POST", "/api/v1/terminal/sessions", { kind: "shell" })).not.toThrow();
    expect(() => validateAPIRequest("POST", "/api/v1/terminal/sessions", { kind: "telnet" })).toThrow("invalid_response");
    expect(() => validateAPIResponse("POST", "/api/v1/terminal/sessions", 200, {})).toThrow("invalid_response");
  });

  it("enforces cross-field constraints declared by the sshc extension", () => {
    expect(() => validateOpenAPISchema("TerminalConnectionProgress", {
      phase: "authenticating", alias: "edge", hostName: "edge", user: "ops", hop: 3, hops: 2,
    })).toThrow("invalid_response");
    expect(() => validateOpenAPISchema("TerminalReconnect", {
      attempt: 3, limit: 2, retryAt: "2026-08-31T12:00:00Z", problem: "network",
    })).toThrow("invalid_response");
  });

  it("accepts strict extended resources and enforces exclusive request fields", () => {
    expect(() => validateOpenAPISchema("TerminalWorkspace", {
      id: "workspace-one",
      name: "Operations",
      layout: { pane: { id: "pane-one", alias: "edge", kind: "ssh" } },
      focusedPaneId: "pane-one",
      createdAt: "2026-08-31T12:00:00Z",
      updatedAt: "2026-08-31T12:01:00Z",
    })).not.toThrow();
    expect(() => validateOpenAPISchema("WorkspaceNode", {})).toThrow("invalid_response");
    expect(() => validateOpenAPISchema("WorkspaceNode", {
      pane: { id: "pane-one", alias: "edge" },
      split: {
        direction: "horizontal", ratio: 50,
        first: { pane: { id: "pane-two", alias: "edge" } },
        second: { pane: { id: "pane-three", alias: "edge" } },
      },
    })).toThrow("invalid_response");
    expect(() => validateOpenAPISchema("Snippet", {
      id: "snippet-one",
      name: "Uptime",
      description: "",
      command: "uptime",
      variables: [],
      createdAt: "2026-08-31T12:00:00Z",
      updatedAt: "2026-08-31T12:01:00Z",
    })).not.toThrow();

    const valid = { snippetId: "snippet-one", aliases: ["edge"], inputs: {}, evidence: "proof", concurrency: 1 };
    expect(() => validateAPIRequest("POST", "/api/v1/snippets/jobs", valid)).not.toThrow();
    expect(() => validateAPIRequest("POST", "/api/v1/snippets/jobs", {
      ...valid,
      command: "uptime",
    })).toThrow("invalid_response");
    expect(() => validateAPIRequest("POST", "/api/v1/snippets/jobs", {
      snippetId: "snippet-one",
      inputs: {}, evidence: "proof", concurrency: 1,
    })).toThrow("invalid_response");
  });

  it("leaves unknown low-level transport paths to their caller", () => {
    const payload = { extension: true };
    expect(validateAPIResponse("GET", "/api/v1/private-extension", 200, payload)).toBe(payload);
    expect(() => validateAPIRequest("POST", "/api/v1/private-extension", payload)).not.toThrow();
  });
});
