import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { TerminalQuickCommands } from "./TerminalQuickCommands";

const mocks = vi.hoisted(() => ({
  library: vi.fn(),
  preview: vi.fn(),
  writeText: vi.fn(),
}));

vi.mock("../snippets/api", async () => {
  const actual = await vi.importActual<typeof import("../snippets/api")>("../snippets/api");
  return { ...actual, snippetsApi: { ...actual.snippetsApi, library: mocks.library } };
});
vi.mock("../features/workspaces/commandApi", () => ({ terminalCommandApi: { preview: mocks.preview } }));
vi.mock("../ui/clipboard", () => ({ clipboard: { readText: vi.fn(), writeText: mocks.writeText } }));

const session = {
  id: "session-one",
  kind: "ssh" as const,
  alias: "edge",
  title: "edge",
  state: "connected" as const,
  problem: "",
  startedAt: "2026-08-29T00:00:00Z",
};

describe("TerminalQuickCommands", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.library.mockResolvedValue({
      startup: [],
      snippets: [{ id: "one", name: "Status", description: "Show status", command: "systemctl status {{unit}}", variables: [{ name: "unit", type: "string", required: true }], createdAt: "", updatedAt: "" }],
    });
    mocks.preview.mockResolvedValue({
      snippetId: "one", evidence: "e", actionToken: "t", actionExpiresAt: "",
      targets: [{ targetId: "session-one", sessionId: "session-one", alias: "edge", title: "edge", command: "systemctl status sshd" }],
    });
    mocks.writeText.mockResolvedValue(undefined);
  });

  it("previews an expanded snippet and inserts it without submitting", async () => {
    const send = vi.fn();
    render(<LanguageProvider><TerminalQuickCommands session={session} onSend={send} onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.type(screen.getByRole("textbox"), "sshd");
    await userEvent.click(screen.getByRole("button", { name: "Preview execution" }));
    expect(await screen.findByText("systemctl status sshd")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Insert" }));
    expect(mocks.preview).toHaveBeenCalledWith(expect.objectContaining({
      snippetId: "one", inputs: { unit: "sshd" }, targets: [{ targetId: "session-one", sessionId: "session-one" }],
    }));
    expect(send).toHaveBeenCalledWith("systemctl status sshd", false);
  });

  it("copies only the server-expanded command", async () => {
    render(<LanguageProvider><TerminalQuickCommands session={session} onSend={() => undefined} onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.type(screen.getByRole("textbox"), "sshd");
    await userEvent.click(screen.getByRole("button", { name: "Preview execution" }));
    await userEvent.click(await screen.findByRole("button", { name: "Copy" }));
    expect(mocks.writeText).toHaveBeenCalledWith("systemctl status sshd");
  });
});
