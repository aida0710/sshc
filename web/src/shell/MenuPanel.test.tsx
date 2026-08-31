import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { MenuPanel, type MenuGroup } from "./MenuPanel";

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
});
