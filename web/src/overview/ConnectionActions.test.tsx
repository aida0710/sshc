import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConnectionActions } from "./ConnectionActions";

describe("ConnectionActions", () => {
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
