import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { LanguageProvider } from "../i18n/context";
import { snippetsApi } from "../snippets/api";
import { CommandPalette } from "./CommandPalette";

vi.mock("../snippets/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../snippets/api")>();
  return { ...actual, snippetsApi: { ...actual.snippetsApi, library: vi.fn() } };
});

const labels = {
  Home: "section.home",
  Menu: "section.menu",
  Connections: "section.connections",
  Terminal: "section.terminal",
  Files: "section.files",
  Snippets: "section.snippets",
  Config: "section.config",
  Keys: "section.keys",
  "Known Hosts": "section.knownHosts",
  "Remote Keys": "section.remoteKeys",
  Diagnostics: "section.diagnostics",
  Passwords: "section.passwords",
  "Key Passphrases": "section.keyPassphrases",
  OTP: "section.otp",
  Settings: "section.settings",
  Sync: "section.sync",
  VPN: "section.vpn",
  History: "section.history",
  License: "section.license",
  About: "section.about",
} as const;

function renderPalette(overrides: Partial<ComponentProps<typeof CommandPalette>> = {}) {
  const props: ComponentProps<typeof CommandPalette> = {
    open: true,
    hosts: [
      { identity: { path: "conf.d/lab.conf", alias: "r540" }, file: { path: "conf.d/lab.conf", absolute: "/home/tester/.ssh/conf.d/lab.conf" }, line: 1, patterns: ["r540"], editable: true },
      { identity: { path: "config", alias: "nas" }, file: { path: "config", absolute: "/home/tester/.ssh/config" }, line: 1, patterns: ["nas"], editable: true },
    ],
    files: [{ file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 }],
    sessions: [],
    unreadBySession: new Set(),
    sectionLabels: labels,
    onClose: vi.fn(),
    onConnect: vi.fn(),
    onOpenHostSettings: vi.fn(),
    onOpenFile: vi.fn(),
    onNavigate: vi.fn(),
    onOpenSnippet: vi.fn(),
    onOpenSession: vi.fn(),
    ...overrides,
  };
  render(<LanguageProvider><CommandPalette {...props} /></LanguageProvider>);
  return props;
}

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets: [{
      id: "update",
      name: "Update packages",
      description: "",
      command: "sudo apt update",
      variables: [],
      createdAt: "2026-08-27T00:00:00Z",
      updatedAt: "2026-08-27T00:00:00Z",
    }],
    startup: [],
  });
});

describe("CommandPalette", () => {
  it("runs an action as well as navigating, and closes once it has", async () => {
    const user = userEvent.setup();
    const run = vi.fn();
    const props = renderPalette({
      commands: [{
        id: "new-connection",
        label: "Create a connection",
        detail: "Open the new connection form",
        search: "new connection create host add",
        run,
      }],
    });

    const search = screen.getByRole("searchbox", { name: "Search sessions, hosts, files, snippets and settings" });
    await user.type(search, "create a conn");
    const option = screen.getByRole("option", { name: /Create a connection/ });
    expect(option).toHaveTextContent("Action");

    await user.click(option);
    expect(run).toHaveBeenCalledOnce();
    expect(props.onClose).toHaveBeenCalled();
  });


  it("searches hosts, files, snippets and settings without persisting the query", async () => {
    const user = userEvent.setup();
    renderPalette();

    const search = screen.getByRole("searchbox", { name: "Search sessions, hosts, files, snippets and settings" });
    expect(screen.getByRole("option", { name: /Connect to r540/ })).toBeInTheDocument();

    await user.type(search, "apt update");
    await waitFor(() => expect(screen.getByRole("option", { name: /Update packages/ })).toBeInTheDocument());
    expect(screen.queryByRole("option", { name: /Connect to r540/ })).toBeNull();
    expect(window.localStorage).toHaveLength(0);
  });

  it("opens the selected result with the keyboard and closes", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const onClose = vi.fn();
    renderPalette({ onConnect, onClose });

    const search = screen.getByRole("searchbox", { name: "Search sessions, hosts, files, snippets and settings" });
    await user.type(search, "r540{Enter}");

    expect(onClose).toHaveBeenCalledOnce();
    expect(onConnect).toHaveBeenCalledWith("r540");
  });

  describe.each([
    { isComposing: true },
    { isComposing: false, keyCode: 229 },
  ])("while the IME reports %j", (composition) => {
    it("leaves selection and execution to the IME until composition ends", async () => {
      const props = renderPalette();
      const search = screen.getByRole("searchbox");
      await waitFor(() => expect(search).toHaveFocus());
      const selected = search.getAttribute("aria-activedescendant");
      for (const key of ["ArrowDown", "ArrowUp", "Enter"]) {
        expect(fireEvent.keyDown(search, { key, ...composition })).toBe(true);
      }
      expect(search).toHaveAttribute("aria-activedescendant", selected);
      expect(props.onClose).not.toHaveBeenCalled();
      expect(props.onConnect).not.toHaveBeenCalled();

      await userEvent.keyboard("{Enter}");
      expect(props.onConnect).toHaveBeenCalledExactlyOnceWith("r540");
      expect(props.onClose).toHaveBeenCalledOnce();
    });

    it("keeps the palette open on composition Escape and closes on the next Escape", async () => {
      const props = renderPalette();
      const search = screen.getByRole("searchbox");
      await waitFor(() => expect(search).toHaveFocus());
      expect(fireEvent.keyDown(search, { key: "Escape", ...composition })).toBe(true);
      expect(props.onClose).not.toHaveBeenCalled();
      expect(search).toHaveFocus();

      await userEvent.keyboard("{Escape}");
      expect(props.onClose).toHaveBeenCalledOnce();
    });
  });

  it("closes when the backdrop is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    renderPalette({ onClose });

    await user.click(document.body);

    expect(onClose).toHaveBeenCalledOnce();
  });

  it("opens host settings from the trailing action without connecting", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const onOpenHostSettings = vi.fn();
    const onClose = vi.fn();
    renderPalette({ onConnect, onOpenHostSettings, onClose });

    const settings = screen.getByRole("button", { name: "Open connection settings for r540" });
    // 設定を開くだけのボタンなので、操作の一覧を出す「…」ではなく歯車で見せる。
    expect(settings.querySelector("[data-icon]")).toHaveAttribute("data-icon", "settings");
    await user.click(settings);

    expect(onClose).toHaveBeenCalledOnce();
    expect(onOpenHostSettings).toHaveBeenCalledWith({ path: "conf.d/lab.conf", alias: "r540" });
    expect(onConnect).not.toHaveBeenCalled();
  });

  it("lists hosts to connect before sessions that are already open", () => {
    renderPalette({
      sessions: [
        { id: "first", kind: "ssh", workspaceId: "default", alias: "r540", title: "r540", startedAt: "2026-08-29T00:00:00Z", state: "connected", problem: "" },
      ],
    });

    const labels = screen.getAllByRole("option").map((option) => option.textContent ?? "");
    const host = labels.findIndex((label) => label.includes("Connect to r540"));
    const session = labels.findIndex((label) => label.includes("connected · r540"));
    expect(host).toBeGreaterThanOrEqual(0);
    expect(session).toBeGreaterThan(host);
  });

  it("jumps to a live session and filters unread notifications without changing its order", async () => {
    const user = userEvent.setup();
    const onOpenSession = vi.fn();
    renderPalette({
      sessions: [
        { id: "first", kind: "ssh", workspaceId: "default", alias: "osaka", title: "First", startedAt: "2026-08-29T00:00:00Z", state: "connected", problem: "" },
        { id: "second", kind: "ssh", workspaceId: "default", alias: "tokyo", title: "Fix login", startedAt: "2026-08-29T00:01:00Z", state: "connected", problem: "", notificationVersion: 1, lastNotification: { title: "", body: "waiting for input", occurredAt: "2026-08-29T00:02:00Z" } },
      ],
      unreadBySession: new Set(["second"]),
      onOpenSession,
    });

    const sessions = screen.getAllByRole("option").filter((option) => /connected · /.test(option.textContent ?? ""));
    expect(sessions[0]).toHaveTextContent("First");
    expect(sessions[1]).toHaveTextContent("Fix login");
    await user.type(screen.getByRole("searchbox"), "@unread{Enter}");

    expect(onOpenSession).toHaveBeenCalledWith("second");
  });

});
