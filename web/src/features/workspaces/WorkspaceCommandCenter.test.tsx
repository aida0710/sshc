import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceCommandCenter, type WorkspaceCommandTarget } from "./WorkspaceCommandCenter";

const snippets = vi.hoisted(() => ({ library: vi.fn() }));
const commands = vi.hoisted(() => ({ preview: vi.fn(), dispatch: vi.fn() }));

vi.mock("../../snippets/api", () => ({ snippetsApi: snippets }));
vi.mock("./commandApi", () => ({ terminalCommandApi: commands }));

const edge: WorkspaceCommandTarget = {
  targetId: "pane-a",
  sessionId: "session-a",
  alias: "edge",
  title: "Primary terminal",
  paneNumber: 1,
  connected: true,
  state: "connected",
};

const preview = {
  snippetId: "",
  evidence: "evidence",
  reviewEvidence: "review-evidence",
  actionToken: "token",
  actionExpiresAt: "2026-08-27T10:00:00Z",
  targets: [
    { targetId: "pane-a", sessionId: "session-a", alias: "edge", title: "Primary terminal", command: "uptime" },
  ],
};

describe("WorkspaceCommandCenter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    snippets.library.mockResolvedValue({ snippets: [], startup: [] });
    commands.preview.mockResolvedValue(preview);
    commands.dispatch.mockResolvedValue({
      results: [{ targetId: "pane-a", sessionId: "session-a", alias: "edge", title: "Primary terminal", status: "delivered" }],
    });
  });

  it("opens as a modal and explains that it writes to the live terminal", async () => {
    const close = vi.fn();
    render(<WorkspaceCommandCenter paneTargets={[edge]} onClose={close} />);

    expect(screen.getByRole("dialog", { name: "Send to connected terminals" })).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText(/working directory, environment, and shell state/)).toBeVisible();
    await userEvent.keyboard("{Escape}");
    expect(close).toHaveBeenCalledTimes(1);
  });

  it("closes when the backdrop is clicked", async () => {
    const close = vi.fn();
    render(<WorkspaceCommandCenter paneTargets={[edge]} onClose={close} />);

    await userEvent.click(document.body);

    expect(close).toHaveBeenCalledOnce();
  });

  it("sends every connected pane even when aliases are duplicated", async () => {
    const user = userEvent.setup();
    const duplicate = { ...edge, targetId: "pane-b", sessionId: "session-b", title: "Second terminal", paneNumber: 2 };
    render(<WorkspaceCommandCenter paneTargets={[edge, duplicate]} onClose={() => undefined} />);

    expect(screen.queryByRole("button", { name: "Once per host" })).toBeNull();
    await user.type(screen.getByLabelText("Command"), "uptime");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));

    await waitFor(() => expect(commands.preview).toHaveBeenCalledWith({
      command: "uptime",
      inputs: {},
      targets: [
        { targetId: "pane-a", sessionId: "session-a" },
        { targetId: "pane-b", sessionId: "session-b" },
      ],
    }));
    expect(screen.getAllByText(/Primary terminal · Pane 1/).length).toBeGreaterThan(0);
    expect(screen.getByText(/Second terminal · Pane 2/)).toBeVisible();
  });

  it("omits panes whose SSH session is not connected", async () => {
    const user = userEvent.setup();
    render(<WorkspaceCommandCenter paneTargets={[
      edge,
      { targetId: "pane-b", alias: "edge", title: "Restoring terminal", paneNumber: 2, connected: false, state: "reconnecting" },
    ]} onClose={() => undefined} />);

    expect(screen.getByText("Not sent (reconnecting)")).toBeVisible();
    await user.type(screen.getByLabelText("Command"), "pwd");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));

    await waitFor(() => expect(commands.preview).toHaveBeenCalledWith({
      command: "pwd",
      inputs: {},
      targets: [{ targetId: "pane-a", sessionId: "session-a" }],
    }));
  });

  it("dispatches the exact request that produced the preview and only reports delivery", async () => {
    const user = userEvent.setup();
    render(<WorkspaceCommandCenter paneTargets={[edge]} onClose={() => undefined} />);
    await user.type(screen.getByLabelText("Command"), "uptime");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    await user.click(await screen.findByRole("button", { name: "Send to 1 terminals" }));

    await waitFor(() => expect(commands.dispatch).toHaveBeenCalledWith(preview, {
      command: "uptime",
      inputs: {},
      targets: [{ targetId: "pane-a", sessionId: "session-a" }],
    }));
    expect(await screen.findByText(/Primary terminal · Pane 1 · Sent/)).toBeVisible();
    expect(screen.getByText(/does not mean the command has finished/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
  });

  it("invalidates a preview when a pane is rebound to another session", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<WorkspaceCommandCenter paneTargets={[edge]} onClose={() => undefined} />);
    await user.type(screen.getByLabelText("Command"), "uptime");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    expect(await screen.findByRole("button", { name: "Send to 1 terminals" })).toBeVisible();

    rerender(<WorkspaceCommandCenter paneTargets={[{ ...edge, sessionId: "replacement-session" }]} onClose={() => undefined} />);

    await waitFor(() => expect(screen.queryByRole("button", { name: "Send to 1 terminals" })).toBeNull());
  });

  it("discards a preview response that arrives after a pane is rebound", async () => {
    let resolvePreview: ((value: typeof preview) => void) | undefined;
    commands.preview.mockReturnValue(new Promise((resolve) => { resolvePreview = resolve; }));
    const user = userEvent.setup();
    const { rerender } = render(<WorkspaceCommandCenter paneTargets={[edge]} onClose={() => undefined} />);
    await user.type(screen.getByLabelText("Command"), "uptime");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    await waitFor(() => expect(commands.preview).toHaveBeenCalledTimes(1));

    rerender(<WorkspaceCommandCenter paneTargets={[{ ...edge, sessionId: "replacement-session" }]} onClose={() => undefined} />);
    resolvePreview?.(preview);

    await waitFor(() => expect(screen.getByRole("button", { name: "Preview execution" })).toBeEnabled());
    expect(screen.queryByRole("button", { name: "Send to 1 terminals" })).toBeNull();
  });

  it("previews secret snippets without exposing the value in the command preview", async () => {
    snippets.library.mockResolvedValue({
      snippets: [{ id: "secret", name: "Deploy", command: "deploy {{token}}", variables: [{ name: "token", type: "secret", required: true }], createdAt: "2026-08-27T00:00:00Z", updatedAt: "2026-08-27T00:00:00Z" }],
      startup: [],
    });
    const user = userEvent.setup();
    render(<WorkspaceCommandCenter paneTargets={[edge]} onClose={() => undefined} />);
    await user.click(screen.getByRole("button", { name: "Saved snippet" }));
    await user.type(await screen.findByLabelText(/token/, { selector: "input" }), "top-secret");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    expect(commands.preview).toHaveBeenCalledWith({
      snippetId: "secret",
      inputs: { token: "top-secret" },
      targets: [{ targetId: "pane-a", sessionId: "session-a" }],
    });
    expect(await screen.findByText("uptime")).toBeVisible();
  });
});
