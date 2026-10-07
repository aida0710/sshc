import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { UpdateApi, UpdateStatus } from "../api/update";
import { UpdateBadge } from "./UpdateBadge";

const available: UpdateStatus = { current: "v1.0.0", latest: "v1.1.0", available: true, canUpdate: true, manager: "install.sh" };
function updater(): UpdateApi {
  return {
    updateStatus: vi.fn().mockResolvedValue(available),
    previewUpdate: vi.fn().mockResolvedValue({ current: "v1.0.0", target: "v1.1.0", manager: "install.sh", actionToken: "confirmed-plan", actionExpiresAt: "2026-10-07T09:00:00Z" }),
    startUpdate: vi.fn().mockResolvedValue({ id: "job-one", target: "v1.1.0", state: "accepted", problem: "" }),
  };
}

describe("Web self update", () => {
  it("shows terminal and transfer disconnection before consuming the preview token", async () => {
    const api = updater();
    render(<UpdateBadge api={api} />);
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));
    const dialog = await screen.findByRole("dialog", { name: "Update sshc?" });
    expect(dialog).toHaveTextContent("disconnects all terminals and file transfers");
    expect(dialog).toHaveTextContent("password-protected vault will lock");
    expect(api.startUpdate).not.toHaveBeenCalled();
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Update sshc" })).toBeEnabled());
    await userEvent.click(within(dialog).getByRole("button", { name: "Update sshc" }));
    expect(api.startUpdate).toHaveBeenCalledExactlyOnceWith("v1.1.0", "confirmed-plan");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("does not start an update after cancellation", async () => {
    const api = updater();
    render(<UpdateBadge api={api} />);
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));
    await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(api.startUpdate).not.toHaveBeenCalled();
  });

  it("requires a new review after the one-time confirmation expires", async () => {
    const api = updater();
    vi.mocked(api.startUpdate).mockRejectedValue(new ApiError("action_token_expired", 403, null));
    render(<UpdateBadge api={api} />);
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Update sshc" })).toBeEnabled());
    await userEvent.click(within(dialog).getByRole("button", { name: "Update sshc" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("review the update again");
    expect(within(dialog).getByRole("button", { name: "Update sshc" })).toBeDisabled();
    expect(api.startUpdate).toHaveBeenCalledTimes(1);
  });

  it("shows restart recovery without offering another installation", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, canUpdate: false, reason: "update_restart_failed", job: { id: "job-one", target: "v1.1.0", state: "restart_required", problem: "update_restart_failed" } });
    render(<UpdateBadge api={api} />);
    expect(await screen.findByRole("status")).toHaveTextContent("sshc engine --replace");
    expect(screen.getByRole("button", { name: "Reload and check result" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
  });

  it("guides Homebrew installations to CLI updates only after the version is opened", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, canUpdate: false, reason: "update_homebrew_unsupported" });
    render(<UpdateBadge api={api} />);
    const version = await screen.findByRole("button", { name: "Version v1.0.0" });
    expect(version).toHaveAttribute("aria-expanded", "false");
    const guidance = screen.getByText(/For Homebrew installations/);
    expect(guidance).not.toBeVisible();
    await userEvent.click(version);
    expect(version).toHaveAttribute("aria-expanded", "true");
    expect(guidance).toBeVisible();
    expect(guidance).toHaveTextContent("update with sshc update in a terminal");
    await userEvent.click(version);
    expect(guidance).not.toBeVisible();
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
    expect(api.previewUpdate).not.toHaveBeenCalled();
    expect(api.startUpdate).not.toHaveBeenCalled();
  });

  it("reports the new engine version and persisted successful result", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.1.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", state: "succeeded", problem: "" } });
    render(<UpdateBadge api={api} />);
    expect(await screen.findByRole("status")).toHaveTextContent("Updated to v1.1.0");
    expect(screen.getByText("Version v1.1.0")).toBeVisible();
  });

  it.each(["update_unmanaged", "update_permission_denied", "update_development_build", "update_windows_unsupported", "update_android_unsupported"])("explains %s under the version and offers no automatic update", async (reason) => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, canUpdate: false, reason });
    render(<UpdateBadge api={api} />);
    const version = await screen.findByRole("button", { name: "Version v1.0.0" });
    const explanation = document.getElementById(version.getAttribute("aria-controls") ?? "");
    expect(explanation).not.toBeVisible();
    await userEvent.click(version);
    expect(explanation).toBeVisible();
    expect(explanation?.textContent).not.toBe("");
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("unavailable for this installation");
  });

  it("keeps the version as plain text when there is nothing to explain", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.0.0", latest: "v1.0.0", available: false, canUpdate: false, reason: "" });
    render(<UpdateBadge api={api} />);
    await screen.findByText("Version v1.0.0");
    await waitFor(() => expect(api.updateStatus).toHaveBeenCalled());
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
