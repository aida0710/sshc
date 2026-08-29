import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";
import type { IconName } from "../ui/icons";
import { MenuPanel, type MenuGroup } from "./MenuPanel";

const labels = {
  Home: "section.home",
  Menu: "section.menu",
  Connections: "section.connections",
  Terminal: "section.terminal",
  Files: "section.files",
  Snippets: "section.snippets",
  Config: "section.config",
  Groups: "section.groups",
  Keys: "section.keys",
  "Known Hosts": "section.knownHosts",
  "Remote Keys": "section.remoteKeys",
  Diagnostics: "section.diagnostics",
  Secrets: "section.secrets",
  Settings: "section.settings",
  Sync: "section.sync",
  History: "section.history",
} satisfies Record<Section, MessageKey>;

const icons = Object.fromEntries(
  Object.keys(labels).map((section) => [section, section === "Menu" ? "menu" : "settings"]),
) as Record<Section, IconName>;

const groups: MenuGroup[] = [
  { label: "shell.navConnections", sections: ["Config", "Groups"] },
  { label: "shell.navMaintenance", sections: ["Settings", "Sync"] },
];

describe("MenuPanel", () => {
  it("shows product areas without a redundant terminal destination", () => {
    render(
      <LanguageProvider initial="en">
        <MenuPanel groups={groups} sectionIcons={icons} sectionLabels={labels} onNavigate={vi.fn()} />
      </LanguageProvider>,
    );

    expect(screen.getByRole("heading", { name: "Menu" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Config" })).toHaveAttribute("href", "/config");
    expect(screen.queryByRole("link", { name: "Open Home" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Open Terminal" })).not.toBeInTheDocument();
  });

  it("uses client navigation for an ordinary click", async () => {
    const onNavigate = vi.fn();
    render(
      <LanguageProvider initial="en">
        <MenuPanel groups={groups} sectionIcons={icons} sectionLabels={labels} onNavigate={onNavigate} />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("link", { name: "Open Sync" }));
    expect(onNavigate).toHaveBeenCalledWith("Sync");
  });
});
