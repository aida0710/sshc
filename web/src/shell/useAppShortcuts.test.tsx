import { renderHook } from "@testing-library/react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { defaultBindings } from "../keyconfig/bindings";
import { useAppShortcuts } from "./useAppShortcuts";

function ignoreNavigation() {}
function ignoreSession() {}

function ShortcutHarness({ ready, openPalette }: { ready: boolean; openPalette: () => void }) {
  useAppShortcuts({
    enabled: ready,
    shortcuts: defaultBindings,
    terminalFace: false,
    orderedSessions: [],
    activeSessionId: null,
    panes: [],
    navigate: ignoreNavigation,
    showSession: ignoreSession,
    openPalette,
    requestSession: ignoreSession,
  });
  return <p>{ready ? "ready" : "starting"}</p>;
}

// IS_REACT_ACT_ENVIRONMENT は、React が act() の外の更新を警告するかを決める。この
// テストはブラウザと同じ順（画面の書き換え → 利用者のキー入力 → 描画後の effect）を
// 作るので、act() で effect まで一度に流さず、その間だけ警告を止める。
type ReactActEnvironment = { IS_REACT_ACT_ENVIRONMENT?: boolean | undefined };

describe("useAppShortcuts", () => {
  // ready の画面が出た直後、描画後の effect が走る前に押された Ctrl+K も
  // パレットを開く。画面は操作できるように見えるので、そこで押したキーを
  // 捨てると、利用者には押しても何も起きなかったように見える。
  it("opens the palette for Ctrl+K pressed as soon as the ready screen is shown", async () => {
    const actEnvironment = globalThis as ReactActEnvironment;
    const previousActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT;
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = false;
    const openPalette = vi.fn();
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    try {
      root.render(<ShortcutHarness ready={false} openPalette={openPalette} />);
      await vi.waitFor(() => expect(container.textContent).toBe("starting"));

      // MutationObserver の知らせは、画面を書き換えた task のすぐあとの microtask で
      // 届く。描画後の effect はそのあとの task なので、ここで押したキーは、
      // effect がまだ走っていない時点のキー入力になる。
      const pressedOnTheReadyScreen = new Promise<void>((pressed) => {
        const observer = new MutationObserver(() => {
          if (container.textContent !== "ready") return;
          observer.disconnect();
          document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true }));
          pressed();
        });
        observer.observe(container, { subtree: true, childList: true, characterData: true });
      });
      root.render(<ShortcutHarness ready openPalette={openPalette} />);
      await pressedOnTheReadyScreen;

      expect(openPalette).toHaveBeenCalledTimes(1);
    } finally {
      root.unmount();
      container.remove();
      actEnvironment.IS_REACT_ACT_ENVIRONMENT = previousActEnvironment;
    }
  });
});

describe("useAppShortcuts on the terminal screen", () => {
  const bindings = { ...defaultBindings, newSession: ["Alt+N"], closeSession: ["Alt+W"], nextPane: ["Alt+]"], previousPane: ["Alt+["] };
  const session = (id: string) => ({ id, kind: "shell" as const, workspaceId: "default", title: id, startedAt: "2026-10-31T00:00:00Z", state: "connected" as const, problem: "" });
  function press(key: string) {
    document.dispatchEvent(new KeyboardEvent("keydown", { key, altKey: true, bubbles: true, cancelable: true }));
  }
  function setUp(panes: string[]) {
    const showSession = vi.fn();
    const requestSession = vi.fn();
    renderHook(() => useAppShortcuts({
      enabled: true,
      shortcuts: bindings,
      terminalFace: true,
      orderedSessions: [session("one"), session("two"), session("three")],
      activeSessionId: "two",
      panes,
      navigate: ignoreNavigation,
      showSession,
      openPalette: ignoreNavigation,
      requestSession,
    }));
    return { showSession, requestSession };
  }

  it("asks the session list for a new session and to close the session shown", () => {
    const { requestSession } = setUp([]);
    press("n");
    press("w");
    expect(requestSession.mock.calls).toEqual([[{ action: "newSession" }], [{ action: "closeSession", sessionId: "two" }]]);
  });

  it("moves between the panes of the split shown, going round at either end", () => {
    const { showSession } = setUp(["one", "two", "three"]);
    press("]");
    press("[");
    expect(showSession.mock.calls).toEqual([["three"], ["one"]]);
  });

  it("leaves the pane keys alone outside a split", () => {
    const { showSession } = setUp(["two"]);
    press("]");
    expect(showSession).not.toHaveBeenCalled();
  });
});
