import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { UpdateApi, UpdateStatus } from "../api/update";
import { LanguageProvider } from "../i18n/context";
import type { MenuGroup } from "./menuEntries";
import { MenuPanel } from "./MenuPanel";

const { signOut } = vi.hoisted(() => ({ signOut: vi.fn() }));
vi.mock("../session/signOut", () => ({ signOut }));

const configuration: MenuGroup = {
  label: "shell.navConnections",
  items: [
    { key: "Config", label: "section.config", description: "menu.configDescription", icon: "config", href: "/config" },
    { key: "Keys", label: "section.keys", description: "menu.keysDescription", icon: "keys", href: "/keys" },
  ],
};
const settings: MenuGroup = {
  label: "section.settings",
  items: [
    { key: "Engine", label: "engine.heading", description: "menu.engineDescription", icon: "engine", href: "/settings/engine" },
    { key: "Terminal", label: "terminal.settingsHeading", description: "menu.terminalDescription", icon: "terminal", href: "/settings/terminal" },
  ],
};
const others: MenuGroup = {
  label: "menu.others",
  items: [
    { key: "Sync", label: "section.sync", description: "menu.syncDescription", icon: "sync", href: "/sync" },
    { key: "About", label: "section.about", description: "menu.aboutDescription", icon: "info", href: "/about" },
  ],
};

function updateApi(status: UpdateStatus): UpdateApi {
  return { checkUpdate: vi.fn(), updateStatus: vi.fn().mockResolvedValue(status), previewUpdate: vi.fn(), startUpdate: vi.fn(), updateLog: vi.fn() };
}

function renderMenu({ onNavigate = vi.fn(), status = { current: "v1.0.0", available: false } }: { onNavigate?: () => void; status?: UpdateStatus } = {}) {
  render(
    <LanguageProvider initial="en">
      <MenuPanel columns={[[configuration], [settings], [others]]} onNavigate={onNavigate} api={updateApi(status)} />
    </LanguageProvider>,
  );
}

describe("MenuPanel", () => {
  it("lists each page with a line on what it is for", () => {
    renderMenu();

    expect(screen.getByRole("heading", { name: "Menu" })).toBeInTheDocument();
    const config = screen.getByRole("link", { name: "Open Config" });
    expect(config).toHaveAttribute("href", "/config");
    expect(config).toHaveTextContent("Edit config files and their Includes");
    expect(screen.getByRole("link", { name: "Open Engine" })).toHaveAttribute("href", "/settings/engine");
    expect(screen.getByRole("link", { name: "Open Terminal" })).toHaveAttribute("href", "/settings/terminal");
    expect(screen.queryByRole("link", { name: "Open Home" })).not.toBeInTheDocument();
  });

  it("keeps only the language under Preferences, since the theme is on Settings → Appearance", () => {
    renderMenu();

    const preferences = screen.getByRole("region", { name: "Preferences" });
    expect(within(preferences).getByLabelText("Language")).toBeInTheDocument();
    expect(screen.queryByLabelText("Theme")).not.toBeInTheDocument();
  });

  it("says on About that a newer version is out, in place of its usual line", async () => {
    renderMenu({ status: { current: "v1.0.0", latest: "v1.1.0", available: true } });

    expect(await screen.findByText("v1.1.0 is available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open About sshc" })).not.toHaveTextContent("Version and updates");
  });

  it("uses client navigation for an ordinary click", async () => {
    const onNavigate = vi.fn();
    renderMenu({ onNavigate });

    await userEvent.click(screen.getByRole("link", { name: "Open Sync" }));
    expect(onNavigate).toHaveBeenCalledWith("/sync");
  });

  it("signs out only after the confirmation and then reloads", async () => {
    const user = userEvent.setup();
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    signOut.mockResolvedValue(undefined);
    renderMenu();

    const signOutButton = screen.getByRole("button", { name: "Sign out of this browser" });
    expect(signOutButton).toHaveAccessibleDescription("Open it again from the URL sshc open prints");
    await user.click(signOutButton);
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
    renderMenu();

    await user.click(screen.getByRole("button", { name: "Sign out of this browser" }));
    await user.click(screen.getByRole("dialog").querySelector("button:last-of-type") as HTMLElement);
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not sign out.");
    expect(reload).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});
