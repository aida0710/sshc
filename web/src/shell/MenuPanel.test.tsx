import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { MenuPanel, type MenuGroup } from "./MenuPanel";

const { signOut } = vi.hoisted(() => ({ signOut: vi.fn() }));
vi.mock("../session/signOut", () => ({ signOut }));

const groups: MenuGroup[] = [
  {
    label: "shell.navConnections",
    items: [
      { key: "Config", label: "section.config", icon: "config", href: "/config" },
      { key: "Groups", label: "section.groups", icon: "groups", href: "/groups" },
    ],
  },
  {
    label: "section.settings",
    items: [
      { key: "Engine", label: "engine.heading", icon: "settings", href: "/settings/engine" },
      { key: "Terminal", label: "terminal.settingsHeading", icon: "terminal", href: "/settings/terminal" },
    ],
  },
  {
    label: "shell.navMaintenance",
    items: [{ key: "Sync", label: "section.sync", icon: "sync", href: "/sync" }],
  },
];

describe("MenuPanel", () => {
  it("shows product areas without a redundant terminal destination", () => {
    render(
      <LanguageProvider initial="en">
        <MenuPanel groups={groups} onNavigate={vi.fn()} />
      </LanguageProvider>,
    );

    const heading = screen.getByRole("heading", { name: "Menu" });
    expect(heading).toBeInTheDocument();
    expect(heading.parentElement?.querySelector("p")).toBeNull();
    expect(screen.getByRole("link", { name: "Open Config" })).toHaveAttribute("href", "/config");
    expect(screen.getByRole("link", { name: "Open Engine" })).toHaveAttribute("href", "/settings/engine");
    expect(screen.getByRole("link", { name: "Open Terminal" })).toHaveAttribute("href", "/settings/terminal");
    expect(screen.queryByRole("link", { name: "Open Home" })).not.toBeInTheDocument();
  });

  it("uses client navigation for an ordinary click", async () => {
    const onNavigate = vi.fn();
    render(
      <LanguageProvider initial="en">
        <MenuPanel groups={groups} onNavigate={onNavigate} />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("link", { name: "Open Sync" }));
    expect(onNavigate).toHaveBeenCalledWith("/sync");
  });

  it("signs out only after the confirmation and then reloads", async () => {
    const user = userEvent.setup();
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    signOut.mockResolvedValue(undefined);
    render(
      <LanguageProvider initial="en">
        <MenuPanel groups={groups} onNavigate={vi.fn()} />
      </LanguageProvider>,
    );

    await user.click(screen.getByRole("button", { name: "Sign out of this browser" }));
    expect(signOut).not.toHaveBeenCalled();
    const dialog = await screen.findByRole("dialog", { name: "Sign out of this browser?" });
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(dialog).not.toBeInTheDocument();
    expect(signOut).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Sign out of this browser" }));
    await user.click(screen.getByRole("dialog").querySelector("button:last-of-type") as HTMLElement);
    await waitFor(() => expect(signOut).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
    vi.unstubAllGlobals();
  });

  it("shows why signing out failed and stays on the page", async () => {
    const user = userEvent.setup();
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    signOut.mockRejectedValue(new Error("offline"));
    render(
      <LanguageProvider initial="en">
        <MenuPanel groups={groups} onNavigate={vi.fn()} />
      </LanguageProvider>,
    );

    await user.click(screen.getByRole("button", { name: "Sign out of this browser" }));
    await user.click(screen.getByRole("dialog").querySelector("button:last-of-type") as HTMLElement);
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not sign out.");
    expect(reload).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});
