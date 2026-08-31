import { beforeEach, describe, expect, it } from "vitest";
import { reduceLayout, restoreLayout } from "./layout";
import { liveWorkspaceStorageKey, loadLiveWorkspace, saveLiveWorkspace } from "./livePersistence";

beforeEach(() => window.sessionStorage.removeItem(liveWorkspaceStorageKey));

describe("live workspace session persistence", () => {
  it("stores only the live pane binding and transient presentation state", () => {
    let layout = restoreLayout({
      split: {
        direction: "horizontal",
        ratio: 65,
        first: { pane: { id: "web", alias: "private-web-alias" } },
        second: { pane: { id: "db", alias: "private-db-alias" } },
      },
    }, "db");
    layout = reduceLayout(layout, { type: "connection-started", paneId: "web", sessionId: "session-web" });
    layout = reduceLayout(layout, { type: "connection-started", paneId: "db", sessionId: "session-db" });

    saveLiveWorkspace(window.sessionStorage, layout, "db", "Operations");

    const raw = window.sessionStorage.getItem(liveWorkspaceStorageKey) ?? "";
    expect(raw).not.toContain("private-web-alias");
    expect(raw).not.toContain("private-db-alias");
    expect(JSON.parse(raw)).toEqual({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 65,
          first: { pane: { id: "web", sessionId: "session-web" } },
          second: { pane: { id: "db", sessionId: "session-db" } },
        },
      },
      focusedPaneId: "db",
      focusModePaneId: "db",
      name: "Operations",
    });
  });

  it("removes stale sessions, collapses their split, and repairs focus", () => {
    window.sessionStorage.setItem(liveWorkspaceStorageKey, JSON.stringify({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 60,
          first: { pane: { id: "web", sessionId: "session-web" } },
          second: {
            split: {
              direction: "vertical",
              ratio: 70,
              first: { pane: { id: "db", sessionId: "stale-db" } },
              second: { pane: { id: "logs", sessionId: "session-logs" } },
            },
          },
        },
      },
      focusedPaneId: "db",
      focusModePaneId: "db",
      name: "Build workers",
    }));

    expect(loadLiveWorkspace(window.sessionStorage, new Set(["session-web", "session-logs"]))).toEqual({
      root: {
        split: {
          direction: "horizontal",
          ratio: 60,
          first: { pane: { id: "web", sessionId: "session-web" } },
          second: { pane: { id: "logs", sessionId: "session-logs" } },
        },
      },
      focusedPaneId: "web",
      focusModePaneId: null,
      name: "Build workers",
    });
  });

  it("discards a snapshot when fewer than two bound sessions remain", () => {
    window.sessionStorage.setItem(liveWorkspaceStorageKey, JSON.stringify({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 50,
          first: { pane: { id: "web", sessionId: "session-web" } },
          second: { pane: { id: "db", sessionId: "stale-db" } },
        },
      },
      focusedPaneId: "web",
      focusModePaneId: null,
    }));

    expect(loadLiveWorkspace(window.sessionStorage, new Set(["session-web"]))).toBeNull();
    expect(window.sessionStorage.getItem(liveWorkspaceStorageKey)).toBeNull();
  });

  it("rejects malformed or duplicated pane bindings", () => {
    window.sessionStorage.setItem(liveWorkspaceStorageKey, JSON.stringify({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 50,
          first: { pane: { id: "same", sessionId: "same-session" } },
          second: { pane: { id: "same", sessionId: "same-session" } },
        },
      },
      focusedPaneId: "same",
      focusModePaneId: null,
    }));

    expect(loadLiveWorkspace(window.sessionStorage, new Set(["same-session"]))).toBeNull();
    expect(window.sessionStorage.getItem(liveWorkspaceStorageKey)).toBeNull();
  });
});
