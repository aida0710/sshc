import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceCommandCenter } from "./WorkspaceCommandCenter";

const api = vi.hoisted(() => ({
  library: vi.fn(),
  previewExecution: vi.fn(),
  startExecution: vi.fn(),
  job: vi.fn(),
  cancel: vi.fn(),
}));

vi.mock("../../snippets/api", () => ({ snippetsApi: api }));

const preview = {
  snippetId: "",
  evidence: "evidence",
  actionToken: "token",
  actionExpiresAt: "2026-08-24T10:00:00Z",
  targets: [
    { targetId: "pane-a", target: { alias: "edge", hostName: "edge.example", user: "aida", port: "22" }, command: "uptime" },
  ],
};

describe("WorkspaceCommandCenter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.library.mockResolvedValue({ snippets: [], startup: [] });
    api.previewExecution.mockResolvedValue(preview);
    api.startExecution.mockResolvedValue({ id: "job", status: "completed", startedAt: "2026-08-24T10:00:00Z", results: [] });
  });

  it("opens as a modal and closes with Escape", async () => {
    const close = vi.fn();
    render(<WorkspaceCommandCenter paneTargets={[{ targetId: "pane-a", alias: "edge", state: "connected" }]} onClose={close} />);

    expect(screen.getByRole("dialog", { name: "Broadcast to terminals" })).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText(/Live keystrokes are never shared/)).toBeVisible();
    await userEvent.keyboard("{Escape}");
    expect(close).toHaveBeenCalledTimes(1);
  });

  it("deduplicates aliases by default and can intentionally execute each pane", async () => {
    const user = userEvent.setup();
    render(<WorkspaceCommandCenter paneTargets={[
      { targetId: "pane-a", alias: "edge", state: "connected" },
      { targetId: "pane-b", alias: "edge", state: "failed" },
    ]} onClose={() => undefined} />);

    await user.type(screen.getByLabelText("Command"), "uptime");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    await waitFor(() => expect(api.previewExecution).toHaveBeenLastCalledWith({
      command: "uptime",
      inputs: {},
      targets: [{ targetId: "pane-a", alias: "edge" }],
    }));

    await user.click(screen.getByRole("button", { name: "Once per pane" }));
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    await waitFor(() => expect(api.previewExecution).toHaveBeenLastCalledWith({
      command: "uptime",
      inputs: {},
      targets: [
        { targetId: "pane-a", alias: "edge" },
        { targetId: "pane-b", alias: "edge" },
      ],
    }));
  });

  it("executes the exact request that produced the preview", async () => {
    const user = userEvent.setup();
    render(<WorkspaceCommandCenter paneTargets={[{ targetId: "pane-a", alias: "edge", state: "connected" }]} onClose={() => undefined} />);
    await user.type(screen.getByLabelText("Command"), "uptime");
    await user.click(screen.getByRole("button", { name: "Preview execution" }));
    await user.click(await screen.findByRole("button", { name: "Run on 1 targets" }));
    await waitFor(() => expect(api.startExecution).toHaveBeenCalledWith(preview, {
      command: "uptime",
      inputs: {},
      targets: [{ targetId: "pane-a", alias: "edge" }],
    }));
  });
});
