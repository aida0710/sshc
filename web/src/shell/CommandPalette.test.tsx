import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { LanguageProvider } from "../i18n/context";
import { snippetsApi } from "../snippets/api";
import { CommandPalette, matches } from "./CommandPalette";

vi.mock("../snippets/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../snippets/api")>();
  return { ...actual, snippetsApi: { ...actual.snippetsApi, library: vi.fn() } };
});

const labels = {
  Home: "section.home",
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
} as const;

function renderPalette(overrides: Partial<ComponentProps<typeof CommandPalette>> = {}) {
  const props: ComponentProps<typeof CommandPalette> = {
    open: true,
    hosts: [
      { identity: { path: "conf.d/lab.conf", alias: "r540" }, file: { path: "conf.d/lab.conf", absolute: "/home/tester/.ssh/conf.d/lab.conf" }, line: 1, patterns: ["r540"], editable: true },
      { identity: { path: "config", alias: "nas" }, file: { path: "config", absolute: "/home/tester/.ssh/config" }, line: 1, patterns: ["nas"], editable: true },
    ],
    files: [{ file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 }],
    sectionLabels: labels,
    onClose: vi.fn(),
    onConnect: vi.fn(),
    onOpenHostSettings: vi.fn(),
    onOpenFile: vi.fn(),
    onNavigate: vi.fn(),
    onOpenSnippet: vi.fn(),
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
      command: "sudo apt update",
      variables: [],
      createdAt: "2026-08-27T00:00:00Z",
      updatedAt: "2026-08-27T00:00:00Z",
    }],
    startup: [],
  });
});

describe("CommandPalette", () => {
  it("searches hosts, files, snippets and settings without persisting the query", async () => {
    const user = userEvent.setup();
    renderPalette();

    const search = screen.getByRole("searchbox", { name: "Search hosts, files, snippets and settings" });
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

    const search = screen.getByRole("searchbox", { name: "Search hosts, files, snippets and settings" });
    await user.type(search, "r540{Enter}");

    expect(onClose).toHaveBeenCalledOnce();
    expect(onConnect).toHaveBeenCalledWith("r540");
  });

  it("opens host settings from the trailing action without connecting", async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn();
    const onOpenHostSettings = vi.fn();
    const onClose = vi.fn();
    renderPalette({ onConnect, onOpenHostSettings, onClose });

    await user.click(screen.getByRole("button", { name: "Open connection settings for r540" }));

    expect(onClose).toHaveBeenCalledOnce();
    expect(onOpenHostSettings).toHaveBeenCalledWith({ path: "conf.d/lab.conf", alias: "r540" });
    expect(onConnect).not.toHaveBeenCalled();
  });

  it("matches every query token irrespective of case", () => {
    expect(matches({ id: "x", kind: "file", label: "SSH Config", detail: "/HOME/AIDA/.ssh/config", search: "settings", action: vi.fn() }, "home config")).toBe(true);
    expect(matches({ id: "x", kind: "file", label: "SSH Config", detail: "/home/aida/.ssh/config", search: "settings", action: vi.fn() }, "host config")).toBe(false);
  });
});
