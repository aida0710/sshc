import { describe, expect, it } from "vitest";
import { paneIDs, reduceLayout, restoreLayout, storeLayout, type StoredNode } from "./layout";

const stored: StoredNode = {
  split: {
    direction: "horizontal",
    ratio: 60,
    first: { pane: { id: "web", alias: "web-prod" } },
    second: { pane: { id: "db", alias: "db-prod" } },
  },
};

describe("workspace layout", () => {
  it("restores every pane as requiring a new connection", () => {
    const restored = restoreLayout(stored, "db");
    expect(restored.focusedPaneId).toBe("db");
    expect(restored.root).toEqual({
      split: {
        direction: "horizontal",
        ratio: 60,
        first: { pane: { id: "web", alias: "web-prod", state: "reconnect_required" } },
        second: { pane: { id: "db", alias: "db-prod", state: "reconnect_required" } },
      },
    });
  });

  it("does not persist a live terminal session ID", () => {
    let state = restoreLayout(stored, "web");
    state = reduceLayout(state, { type: "connection-started", paneId: "web", sessionId: "live-session" });
    expect(storeLayout(state)).toEqual({ layout: stored, focusedPaneId: "web" });
  });

  it("splits a pane and focuses the new connection", () => {
    const initial = restoreLayout({ pane: { id: "web", alias: "web-prod" } }, "web");
    const next = reduceLayout(initial, {
      type: "split",
      paneId: "web",
      direction: "vertical",
      pane: { id: "logs", alias: "logs-prod" },
    });
    expect(next.focusedPaneId).toBe("logs");
    expect(paneIDs(next.root)).toEqual(["web", "logs"]);
    expect(next.root.split?.ratio).toBe(50);
  });

  it("collapses a split when a pane closes", () => {
    const next = reduceLayout(restoreLayout(stored, "db"), { type: "close", paneId: "db" });
    expect(next).toEqual({
      root: { pane: { id: "web", alias: "web-prod", state: "reconnect_required" } },
      focusedPaneId: "web",
    });
  });

  it("keeps the final pane and rejects duplicate pane IDs", () => {
    const initial = restoreLayout({ pane: { id: "web", alias: "web-prod" } }, "web");
    expect(reduceLayout(initial, { type: "close", paneId: "web" })).toBe(initial);
    expect(
      reduceLayout(initial, {
        type: "split",
        paneId: "web",
        direction: "horizontal",
        pane: { id: "web", alias: "another" },
      }),
    ).toBe(initial);
  });

  it("drops all runtime bindings after the engine restarts", () => {
    let state = restoreLayout(stored, "web");
    state = reduceLayout(state, { type: "connection-started", paneId: "web", sessionId: "first" });
    state = reduceLayout(state, { type: "connection-failed", paneId: "db", problem: "offline" });
    const restarted = reduceLayout(state, { type: "engine-restarted" });
    expect(restarted.root).toEqual(restoreLayout(stored, "web").root);
  });

  it("clamps a resized split to the persisted contract", () => {
    const state = restoreLayout(stored, "web");
    expect(reduceLayout(state, { type: "resize-split", path: [], ratio: 2 }).root.split?.ratio).toBe(10);
    expect(reduceLayout(state, { type: "resize-split", path: [], ratio: 98 }).root.split?.ratio).toBe(90);
  });
});
