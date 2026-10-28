import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageHeader } from "./page";
import { PageChromeContext } from "./pageChrome";

function viewport(phone: boolean) {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    matches: phone,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("PageHeader", () => {
  it("puts its actions in the top bar on a phone, where its name is hidden", () => {
    viewport(true);
    const topBar = document.createElement("div");
    document.body.append(topBar);
    render(
      <PageChromeContext.Provider value={{ inspectorToggle: null, phoneActionsSlot: topBar, phoneBackSlot: null }}>
        <PageHeader title="SSH Keys" actions={<button type="button">鍵を作成</button>} />
      </PageChromeContext.Provider>,
    );

    expect(within(topBar).getByRole("button", { name: "鍵を作成" })).toBeInTheDocument();
    expect(within(screen.getByRole("banner")).queryByRole("button", { name: "鍵を作成" })).toBeNull();
    topBar.remove();
  });

  it("takes no room on a phone once its name is for screen readers and its actions are in the top bar", () => {
    viewport(true);
    const topBar = document.createElement("div");
    document.body.append(topBar);
    render(
      <PageChromeContext.Provider value={{ inspectorToggle: null, phoneActionsSlot: topBar, phoneBackSlot: null }}>
        <PageHeader title="Snippets" meta="1 saved" actions={<button type="button">Add snippet</button>} className="py-3" />
      </PageChromeContext.Provider>,
    );

    const header = screen.getByRole("banner");
    expect(header).toHaveClass("sr-only");
    expect(header).not.toHaveClass("py-3");
    expect(within(header).getByRole("heading", { name: "Snippets" })).toBeInTheDocument();
    topBar.remove();
  });

  it("keeps its room on a phone for a description under the hidden name", () => {
    viewport(true);
    render(
      <PageChromeContext.Provider value={{ inspectorToggle: null, phoneActionsSlot: null, phoneBackSlot: null }}>
        <PageHeader title="VPN" description="Requires Docker on this machine." className="py-3" />
      </PageChromeContext.Provider>,
    );

    expect(screen.getByRole("banner")).toHaveClass("py-3");
  });

  it("keeps its actions beside a name shown on a phone", () => {
    viewport(true);
    const topBar = document.createElement("div");
    document.body.append(topBar);
    render(
      <PageChromeContext.Provider value={{ inspectorToggle: null, phoneActionsSlot: topBar, phoneBackSlot: null }}>
        <PageHeader title="Terminal" titleOnPhone actions={<button type="button">保存</button>} />
      </PageChromeContext.Provider>,
    );

    expect(within(screen.getByRole("banner")).getByRole("button", { name: "保存" })).toBeInTheDocument();
    expect(topBar).toBeEmptyDOMElement();
    topBar.remove();
  });

  it("shows the inspector's toggle first among its actions on a wide screen", () => {
    viewport(false);
    render(
      <PageChromeContext.Provider value={{ inspectorToggle: <button type="button">鍵の詳細</button>, phoneActionsSlot: null, phoneBackSlot: null }}>
        <PageHeader title="SSH Keys" actions={<button type="button">鍵を作成</button>} />
      </PageChromeContext.Provider>,
    );

    const buttons = within(screen.getByRole("banner")).getAllByRole("button");
    expect(buttons.map((button) => button.textContent)).toEqual(["鍵の詳細", "鍵を作成"]);
  });
});
