import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { LanguageProvider } from "../i18n/context";
import { TerminalQuickCommands } from "./TerminalQuickCommands";

const mocks = vi.hoisted(() => ({
  library: vi.fn(),
  create: vi.fn(),
  preview: vi.fn(),
  dispatch: vi.fn(),
  writeText: vi.fn(),
}));

vi.mock("../snippets/api", async () => {
  const actual = await vi.importActual<typeof import("../snippets/api")>("../snippets/api");
  return { ...actual, snippetsApi: { ...actual.snippetsApi, library: mocks.library, create: mocks.create } };
});
vi.mock("../features/workspaces/commandApi", () => ({ terminalCommandApi: { preview: mocks.preview, dispatch: mocks.dispatch } }));
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
      snippetId: "one", evidence: "e", reviewEvidence: "r", actionToken: "t", actionExpiresAt: "",
      targets: [{ targetId: "session-one", sessionId: "session-one", alias: "edge", title: "edge", command: "systemctl status sshd" }],
    });
    mocks.writeText.mockResolvedValue(undefined);
    mocks.dispatch.mockResolvedValue({ results: [] });
    mocks.create.mockResolvedValue({ id: "saved", name: "Deploy", command: "deploy --dry-run", variables: [], createdAt: "", updatedAt: "" });
  });

  it("previews an expanded snippet and inserts it without submitting", async () => {
    render(<LanguageProvider><TerminalQuickCommands session={session} onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.type(screen.getByRole("textbox"), "sshd");
    await userEvent.click(screen.getByRole("button", { name: "Preview execution" }));
    expect(await screen.findByText("systemctl status sshd")).toBeVisible();
    expect(screen.getByText(/a password or passphrase prompt/)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Insert" }));
    expect(mocks.preview).toHaveBeenNthCalledWith(2, expect.objectContaining({
      snippetId: "one", issueAction: true, submit: false, expectedReviewEvidence: "r", inputs: { unit: "sshd" }, targets: [{ targetId: "session-one", sessionId: "session-one" }],
    }));
    expect(mocks.dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ evidence: "e", actionToken: "t" }),
      expect.objectContaining({ snippetId: "one", inputs: { unit: "sshd" } }),
      false,
    );
  });

  it("copies only the server-expanded command", async () => {
    render(<LanguageProvider><TerminalQuickCommands session={session} onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.type(screen.getByRole("textbox"), "sshd");
    await userEvent.click(screen.getByRole("button", { name: "Preview execution" }));
    await userEvent.click(await screen.findByRole("button", { name: "Copy" }));
    expect(mocks.writeText).toHaveBeenCalledWith("systemctl status sshd");
  });

  it("issues an action token only when run is chosen and never reveals the command", async () => {
    render(<LanguageProvider><TerminalQuickCommands session={session} onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.type(screen.getByRole("textbox"), "sshd");
    await userEvent.click(screen.getByRole("button", { name: "Preview execution" }));
    await userEvent.click(await screen.findByRole("button", { name: "Run" }));
    expect(mocks.preview).toHaveBeenNthCalledWith(1, expect.objectContaining({ issueAction: false }));
    expect(mocks.preview).toHaveBeenNthCalledWith(2, expect.objectContaining({ issueAction: true, submit: true, expectedReviewEvidence: "r" }));
    expect(mocks.dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ evidence: "e", actionToken: "t" }),
      expect.objectContaining({ snippetId: "one", inputs: { unit: "sshd" } }),
      true,
    );
    expect(mocks.preview).toHaveBeenCalledTimes(2);
  });

  it("refreshes a changed preview and requires another click before delivery", async () => {
    const oldPreview = {
      snippetId: "one", evidence: "old-e", reviewEvidence: "old-r", actionToken: "", actionExpiresAt: "",
      targets: [{ targetId: "session-one", sessionId: "session-one", alias: "edge", title: "edge", command: "systemctl status sshd" }],
    };
    const updatedPreview = {
      snippetId: "one", evidence: "new-e", reviewEvidence: "new-r", actionToken: "", actionExpiresAt: "",
      targets: [{ targetId: "session-one", sessionId: "session-one", alias: "edge", title: "edge", command: "curl example.invalid" }],
    };
    mocks.preview
      .mockResolvedValueOnce(oldPreview)
      .mockRejectedValueOnce(new ApiError("terminal_command_preview_changed", 409, null))
      .mockResolvedValueOnce(updatedPreview);

    render(<LanguageProvider><TerminalQuickCommands session={session} onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.type(screen.getByRole("textbox"), "sshd");
    await userEvent.click(screen.getByRole("button", { name: "Preview execution" }));
    await userEvent.click(await screen.findByRole("button", { name: "Run" }));

    expect(await screen.findByText("curl example.invalid")).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent("The snippet or pane changed");
    expect(mocks.dispatch).not.toHaveBeenCalled();
  });

  it("saves selected terminal text as a reusable snippet", async () => {
    render(<LanguageProvider><TerminalQuickCommands session={session} initialCommand="deploy --dry-run" onClose={() => undefined} /></LanguageProvider>);
    await screen.findByRole("option", { name: "Status" });
    await userEvent.click(screen.getByRole("button", { name: "Save selection as snippet" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Snippet name" }), "Deploy");
    await userEvent.click(screen.getByRole("button", { name: "Save snippet" }));

    expect(mocks.create).toHaveBeenCalledWith({ name: "Deploy", command: "deploy --dry-run", description: "", variables: [] });
    expect(await screen.findByText("Snippet saved.")).toBeVisible();
  });
});
