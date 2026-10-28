import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import type { UpdateApi, UpdateStatus } from "../api/update";
import { UpdateBadge } from "./UpdateBadge";
import { UpdateControls } from "./UpdateControls";

const available: UpdateStatus = { current: "v1.0.0", latest: "v1.1.0", available: true, canUpdate: true, manager: "install.sh" };
function updater(): UpdateApi {
  return {
    checkUpdate: vi.fn(), updateStatus: vi.fn().mockResolvedValue(available),
    previewUpdate: vi.fn().mockResolvedValue({ current: "v1.0.0", target: "v1.1.0", manager: "install.sh", actionToken: "confirmed-plan" }),
    startUpdate: vi.fn().mockResolvedValue({ id: "job-one", target: "v1.1.0", state: "accepted", problem: "" }),
    updateLog: vi.fn(),
  };
}

function localSessions(sessions: TerminalSession[] = []): Pick<TerminalSessionsApi, "terminalSessions"> {
  return { terminalSessions: vi.fn().mockResolvedValue({ sessions, maxSessions: 12 }) };
}

// The update is confirmed on About; these tests give the controls the status the page loaded.
async function renderControls(api: UpdateApi, sessions = localSessions()) {
  render(<UpdateControls status={await api.updateStatus()} api={api} sessions={sessions} onStart={vi.fn()} />);
}

describe("Web self update", () => {
  afterEach(() => localStorage.clear());

  it("shows terminal and transfer disconnection before consuming the preview token", async () => {
    const api = updater();
    await renderControls(api);
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

  it("names the running terminals of this machine that the update ends", async () => {
    const running: TerminalSession = { id: "one", kind: "ssh", alias: "osaka", workspaceId: "default", title: "API認証の修正", startedAt: "2026-10-28T09:00:00Z", state: "connected", problem: "" };
    const ended: TerminalSession = { ...running, id: "two", title: "build", state: "exited", exited: { code: 0, signal: "", at: "2026-10-28T09:10:00Z" } };
    await renderControls(updater(), localSessions([running, ended]));
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));

    const listed = await within(await screen.findByRole("dialog", { name: "Update sshc?" })).findByRole("list", { name: "Sessions that end" });
    expect(within(listed).getAllByRole("listitem").map((row) => row.textContent)).toEqual(["API認証の修正osaka"]);
  });

  it("reviews the PowerShell installation before updating Windows", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, manager: "install.ps1" });
    vi.mocked(api.previewUpdate).mockResolvedValue({ current: "v1.0.0", target: "v1.1.0", manager: "install.ps1", actionToken: "windows-plan" });
    await renderControls(api);
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(dialog).toHaveTextContent("install.ps1"));
    expect(api.startUpdate).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole("button", { name: "Update sshc" }));
    expect(api.startUpdate).toHaveBeenCalledExactlyOnceWith("v1.1.0", "windows-plan");
  });

  it("does not start an update after cancellation", async () => {
    const api = updater();
    await renderControls(api);
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));
    await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(api.startUpdate).not.toHaveBeenCalled();
  });

  it("requires a new review after the one-time confirmation expires", async () => {
    const api = updater();
    vi.mocked(api.startUpdate).mockRejectedValue(new ApiError("action_token_expired", 403, null));
    await renderControls(api);
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
    expect(await within(await screen.findByRole("dialog", { name: "sshc update" })).findByRole("status")).toHaveTextContent("sshc engine --replace");
    expect(screen.getByRole("button", { name: "Reload and check result" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
  });

  it("reviews Homebrew's formula update before starting it", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, manager: "homebrew" });
    vi.mocked(api.previewUpdate).mockResolvedValue({ current: "v1.0.0", target: "v1.1.0", manager: "homebrew", actionToken: "confirmed-plan" });
    await renderControls(api);
    await userEvent.click(await screen.findByRole("button", { name: "Update sshc" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(/A newer version may be installed/);
    expect(api.startUpdate).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole("button", { name: "Update sshc" }));
    expect(api.startUpdate).toHaveBeenCalledExactlyOnceWith("v1.1.0", "confirmed-plan");
  });

  it("reports the installed Homebrew version when it is newer than the preview", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.2.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", installedVersion: "v1.2.0", state: "succeeded", problem: "" } });
    render(<UpdateBadge api={api} />);
    expect(await within(await screen.findByRole("dialog", { name: "sshc update" })).findByRole("status")).toHaveTextContent("Updated to v1.2.0");
    expect(screen.getByRole("link", { name: "Version v1.2.0" })).toBeVisible();
  });

  it("follows a started update in a dialog until its result", async () => {
    const api = updater();
    vi.mocked(api.updateStatus)
      .mockResolvedValueOnce({ current: "v1.0.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", state: "restarting", problem: "" } })
      .mockResolvedValueOnce({ current: "v1.0.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", state: "restarting", problem: "" } })
      .mockResolvedValue({ current: "v1.1.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", state: "succeeded", problem: "" } });
    render(<UpdateBadge api={api} />);
    const dialog = await screen.findByRole("dialog", { name: "sshc update" });
    expect(within(dialog).getByRole("status")).toHaveTextContent("Restarting sshc");
    await waitFor(() => expect(within(dialog).getByRole("status")).toHaveTextContent("Updated to v1.1.0"), { timeout: 4000 });
  });

  it("shows an update result once and not on later loads", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.1.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", state: "succeeded", problem: "" } });
    const first = render(<UpdateBadge api={api} />);
    const dialog = await screen.findByRole("dialog", { name: "sshc update" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByText(/Updated to/)).not.toBeInTheDocument();
    first.unmount();

    render(<UpdateBadge api={api} />);
    await screen.findByRole("link", { name: "Version v1.1.0" });
    await waitFor(() => expect(api.updateStatus).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("does not show an earlier success after sshc was updated another way", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.3.0", available: false, canUpdate: false, job: { id: "job-one", target: "v1.1.0", state: "succeeded", problem: "" } });
    render(<UpdateBadge api={api} />);
    await screen.findByRole("link", { name: "Version v1.3.0" });
    await waitFor(() => expect(api.updateStatus).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("folds the notice of a release under the version until a newer release", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.1.0" });
    const first = render(<UpdateBadge api={api} />);
    const version = await screen.findByRole("button", { name: "Toggle update notice" });
    expect(version).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("link", { name: /v1.1.0 is available/ })).toBeVisible();
    await userEvent.click(version);
    expect(screen.queryByRole("link", { name: /v1.1.0 is available/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
    first.unmount();

    render(<UpdateBadge api={api} />);
    expect(await screen.findByRole("button", { name: "Toggle update notice" })).toHaveAttribute("aria-expanded", "false");
    cleanup();

    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, latest: "v1.2.0", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.2.0" });
    render(<UpdateBadge api={api} />);
    expect(await screen.findByRole("button", { name: "Toggle update notice" })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("link", { name: /v1.2.0 is available/ })).toBeVisible();
  });

  it("asks to refresh Homebrew when the installation did not update", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, manager: "homebrew", job: { id: "job-one", target: "v1.1.0", state: "failed", problem: "update_homebrew_not_updated" } });
    render(<UpdateBadge api={api} />);
    expect(await within(await screen.findByRole("dialog", { name: "sshc update" })).findByRole("status")).toHaveTextContent("brew update");
    expect(screen.getByRole("link", { name: "Version v1.0.0" })).toBeVisible();
    expect(api.startUpdate).not.toHaveBeenCalled();
  });

  it.each(["update_unmanaged", "update_permission_denied", "update_development_build", "update_android_unsupported"])("explains %s under the version and offers no automatic update", async (reason) => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ ...available, available: false, canUpdate: false, reason });
    render(<UpdateBadge api={api} />);
    const version = await screen.findByRole("button", { name: "Toggle update notice" });
    const explanation = document.getElementById(version.getAttribute("aria-controls") ?? "");
    expect(explanation).not.toBeVisible();
    await userEvent.click(version);
    expect(explanation).toBeVisible();
    expect(explanation?.textContent).not.toBe("");
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("unavailable for this installation");
  });

  it("links the version to About when there is nothing to explain", async () => {
    const api = updater();
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.0.0", latest: "v1.0.0", available: false, canUpdate: false, reason: "" });
    render(<UpdateBadge api={api} />);
    await screen.findByRole("link", { name: "Version v1.0.0" });
    await waitFor(() => expect(api.updateStatus).toHaveBeenCalled());
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
