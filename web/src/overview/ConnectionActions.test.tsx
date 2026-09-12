import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConnectionActions } from "./ConnectionActions";

describe("ConnectionActions", () => {
  it("portals the menu and keeps it inside a short visual viewport", async () => {
    const viewport = Object.assign(new EventTarget(), { offsetLeft: 0, offsetTop: 100, width: 390, height: 240 });
    vi.stubGlobal("visualViewport", viewport);
    let anchorTop = 280;
    const rectangle = vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      if (this.getAttribute("role") === "menu") return { width: 224, height: 104 } as DOMRect;
      return { left: 336, right: 380, top: anchorTop, bottom: anchorTop + 44, width: 44, height: 44 } as DOMRect;
    });
    try {
      render(<ConnectionActions alias="database" path="config" busy={false} onOpenSettings={vi.fn()} onConnect={vi.fn()} />);
      await userEvent.click(screen.getByRole("button", { name: "Actions for database" }));
      const menu = screen.getByRole("menu");
      expect(menu.parentElement).toBe(document.body);
      expect(menu).toHaveStyle({ left: "156px", top: "172px", maxHeight: "224px", maxWidth: "374px" });
      anchorTop = 400;
      fireEvent.scroll(window);
      expect(menu).toHaveStyle({ top: "228px" });
      anchorTop = 104;
      viewport.dispatchEvent(new Event("resize"));
      expect(menu).toHaveStyle({ top: "152px" });
    } finally {
      rectangle.mockRestore();
      vi.unstubAllGlobals();
    }
  });

  it("opens settings for the exact config identity without connecting", async () => {
    const openSettings = vi.fn();
    const connect = vi.fn();
    render(
      <ConnectionActions
        alias="database"
        path="connections/work.conf"
        busy={false}
        onOpenSettings={openSettings}
        onConnect={connect}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Actions for database" });
    expect(trigger.querySelector("use")).toHaveAttribute("href", "#icon-moreHorizontal");
    expect(trigger).not.toHaveTextContent("…");
    await userEvent.click(trigger);
    expect(connect).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("menuitem", { name: "Open connection settings" }));
    expect(openSettings).toHaveBeenCalledWith(
      "/connections/servers?path=connections%2Fwork.conf&host=database&panel=basic",
    );
    expect(connect).not.toHaveBeenCalled();
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("connects only from the explicit menu item and closes the menu", async () => {
    const connect = vi.fn();
    render(
      <ConnectionActions
        alias="database"
        path="config"
        busy={false}
        onOpenSettings={vi.fn()}
        onConnect={connect}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Actions for database" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Connect" }));
    expect(connect).toHaveBeenCalledOnce();
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("closes on Escape and an outside pointer action", async () => {
    render(
      <div>
        <ConnectionActions
          alias="database"
          path="config"
          busy={false}
          onOpenSettings={vi.fn()}
          onConnect={vi.fn()}
        />
        <button type="button">Outside</button>
      </div>,
    );

    const trigger = screen.getByRole("button", { name: "Actions for database" });
    await userEvent.click(trigger);
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();

    await userEvent.click(trigger);
    fireEvent.pointerDown(screen.getByRole("button", { name: "Outside" }));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("moves through enabled menu items with the keyboard", async () => {
    const user = userEvent.setup();
    render(
      <ConnectionActions
        alias="database"
        path="config"
        busy={false}
        onOpenSettings={vi.fn()}
        onConnect={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Actions for database" }));
    const settings = screen.getByRole("menuitem", { name: "Open connection settings" });
    const connect = screen.getByRole("menuitem", { name: "Connect" });
    expect(settings).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(connect).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(settings).toHaveFocus();
    await user.keyboard("{End}");
    expect(connect).toHaveFocus();
    await user.keyboard("{Home}");
    expect(settings).toHaveFocus();
  });

  it("keeps settings available while disabling a duplicate connection", async () => {
    render(
      <ConnectionActions
        alias="database"
        path="config"
        busy
        onOpenSettings={vi.fn()}
        onConnect={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Actions for database" }));
    expect(screen.getByRole("menuitem", { name: "Open connection settings" })).toBeEnabled();
    expect(screen.getByRole("menuitem", { name: "Connect" })).toBeDisabled();
  });
});
