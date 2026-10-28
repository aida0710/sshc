import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { describe, expect, it, vi } from "vitest";
import type { About } from "../api/about";
import { ApiError } from "../api/client";
import type { UpdateApi, UpdateStatus } from "../api/update";
import { updateLog } from "../testing/updateLog";
import { AboutPage } from "./AboutPage";
import { UpdateBadge } from "../shell/UpdateBadge";

const about: About = {
  version: "v1.0.0", revision: "abcdef0123456789abcdef0123456789abcdef0123",
  commitTime: "2026-10-17T01:00:00Z", modified: false,
  goVersion: "go1.26.9", os: "linux", arch: "amd64", manager: "homebrew", executable: "/opt/homebrew/bin/sshc",
};
const status: UpdateStatus = {
  current: "v1.0.0", latest: "v1.2.0", available: true, canUpdate: true, manager: "homebrew",
  checkedAt: "2026-10-19T01:00:00Z", publishedAt: "2026-10-18T01:00:00Z",
  releases: [
    { version: "v1.2.0", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.2.0", body: "# Notes\n<script>alert('test')</script>\n  <img src=x onerror=alert(1)>", publishedAt: "2026-10-18T01:00:00Z" },
    { version: "v1.1.0", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.1.0", body: "Intermediate release" },
    { version: "v1.0.0", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.0.0", body: "Current release notes" },
  ],
};
function updates(initial: UpdateStatus = status): UpdateApi {
  return {
    updateStatus: vi.fn().mockResolvedValue(initial), checkUpdate: vi.fn().mockResolvedValue(initial), previewUpdate: vi.fn(), startUpdate: vi.fn(),
    updateLog: vi.fn().mockRejectedValue(new ApiError("update_log_missing", 404, null)),
  };
}
const api = { about: vi.fn().mockResolvedValue(about) };

describe("AboutPage", () => {
  it("shows the running build, update information and release bodies as plain text", async () => {
    const { container } = render(<AboutPage api={api} updates={updates()} />);
    expect(await screen.findByText("/opt/homebrew/bin/sshc")).toBeVisible();
    expect(screen.getByRole("link", { name: "abcdef0" })).toHaveAttribute("href", `https://github.com/aida0710/sshc/commit/${about.revision}`);
    expect(screen.getByText("Homebrew")).toBeVisible();
    expect(screen.getByText("linux / amd64")).toBeVisible();
    expect(screen.getByText("go1.26.9")).toBeVisible();
    expect(await screen.findByText("sshc v1.2.0 is available.")).toBeVisible();
    expect(screen.getByText("Last checked")).toBeVisible();
    const notes = container.querySelectorAll("details");
    expect(notes).toHaveLength(3);
    expect(notes[0]).toHaveAttribute("open");
    expect(notes[1]).not.toHaveAttribute("open");
    expect(notes[2]).not.toHaveAttribute("open");
    expect(within(notes[2] as HTMLElement).getByText("Current version")).toBeVisible();
    const body = notes[0]?.querySelector(".whitespace-pre-wrap");
    expect(body?.textContent).toBe(status.releases?.[0]?.body);
    expect(body?.querySelector("script, img, h1")).toBeNull();
    expect(within(notes[0] as HTMLElement).getByRole("link", { name: "View on GitHub" })).toHaveAttribute("href", status.releases?.[0]?.pageUrl);
  });

  it("checks for updates on opening and shares the result with the sidebar", async () => {
    const update = updates({ current: "v1.0.0", available: false });
    let finish: (status: UpdateStatus) => void = () => {};
    vi.mocked(update.checkUpdate).mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    render(<StrictMode><UpdateBadge api={update} /><AboutPage api={api} updates={update} /></StrictMode>);
    const check = await screen.findByRole("button", { name: "Checking…" });
    expect(check).toBeDisabled();
    // The stored result is not shown as the answer until the check returns.
    expect(screen.queryByText(/is the latest/)).not.toBeInTheDocument();
    const log = screen.getByRole("log", { name: "Check and update progress" });
    expect(within(log).getAllByRole("listitem").map((line) => line.textContent)).toEqual([expect.stringContaining("Checking GitHub for the latest release…")]);
    await act(async () => finish({ ...status, pageUrl: status.releases?.[0]?.pageUrl ?? "" }));
    expect(update.checkUpdate).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("link", { name: /v1.2.0 is available/ })).toBeVisible();
    expect(within(log).getByText("The latest release on GitHub is v1.2.0.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Check for updates" })).toBeEnabled();
  });

  it("does not claim the stored version is latest when the opening check fails", async () => {
    const update = updates({ ...status, latest: "v1.0.0", available: false, canUpdate: false });
    vi.mocked(update.checkUpdate).mockRejectedValue(new Error("offline"));
    render(<AboutPage api={api} updates={update} />);
    await within(await screen.findByRole("log")).findByText("Could not check for the latest release. The sshc engine did not return a result.");
    expect(screen.queryByText("No newer version is available. The running v1.0.0 is the latest.")).not.toBeInTheDocument();
  });

  it("waits for the job status before checking for a newer version", async () => {
    const update = updates();
    let finishStatus: (status: UpdateStatus) => void = () => {};
    vi.mocked(update.updateStatus).mockImplementation(() => new Promise((resolve) => { finishStatus = resolve; }));
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByText("/opt/homebrew/bin/sshc")).toBeVisible();
    expect(update.checkUpdate).not.toHaveBeenCalled();
    await act(async () => finishStatus(status));
    expect(update.checkUpdate).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("sshc v1.2.0 is available.")).toBeVisible();
  });

  it.each(["accepted", "installing", "restarting"] as const)("does not check on opening while an update is %s, even after it finishes", async (state) => {
    const job = { id: "active-job", target: "v1.2.0", state, problem: "" };
    const update = updates();
    vi.mocked(update.updateStatus)
      .mockResolvedValueOnce({ ...status, canUpdate: false, job })
      .mockResolvedValue({ ...status, job: { ...job, state: "succeeded" } });
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByText("Updated to v1.2.0.")).toBeVisible();
    expect(update.checkUpdate).not.toHaveBeenCalled();
    expect(screen.queryByText("sshc v1.2.0 is available.")).not.toBeInTheDocument();
  });

  it("hides the previous latest-version claim during a recheck and after a request failure", async () => {
    const update = updates({ ...status, latest: "v1.0.0", available: false, canUpdate: false });
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByText("No newer version is available. The running v1.0.0 is the latest.")).toBeVisible();
    let failCheck: (reason: Error) => void = () => {};
    vi.mocked(update.checkUpdate).mockImplementation(() => new Promise((_, reject) => { failCheck = reject; }));
    await userEvent.click(screen.getByRole("button", { name: "Check for updates" }));
    expect(screen.getByRole("button", { name: "Checking…" })).toBeDisabled();
    expect(screen.queryByText(/is the latest/)).not.toBeInTheDocument();
    await act(async () => failCheck(new Error("offline")));
    expect(screen.getByText("Could not check for the latest release. The sshc engine did not return a result.")).toBeVisible();
    expect(screen.queryByText(/is the latest/)).not.toBeInTheDocument();
  });

  it("adds a line for every check, so a repeated check with the same result is still visible", async () => {
    const update = updates({ ...status, latest: "v1.0.0", available: false, canUpdate: false });
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByText("No newer version is available. The running v1.0.0 is the latest.")).toBeVisible();
    const check = screen.getByRole("button", { name: "Check for updates" });
    await userEvent.click(check);
    await waitFor(() => expect(check).toBeEnabled());
    await userEvent.click(check);
    await waitFor(() => expect(check).toBeEnabled());
    const lines = within(screen.getByRole("log")).getAllByRole("listitem");
    expect(lines.map((line) => line.querySelector("span")?.textContent)).toEqual([
      "Checking GitHub for the latest release…", "The latest release on GitHub is v1.0.0.",
      "Checking GitHub for the latest release…", "The latest release on GitHub is v1.0.0.",
      "Checking GitHub for the latest release…", "The latest release on GitHub is v1.0.0.",
    ]);
    expect(lines[0]?.querySelector("time")).toHaveAttribute("datetime");
  });

  it("leaves out the build rows it cannot fill, as for a Homebrew build without .git, and keeps the installation guidance", async () => {
    const missing: About = { ...about, revision: "", executable: "", manager: "unknown" };
    delete missing.commitTime;
    delete missing.modified;
    render(<AboutPage api={{ about: vi.fn().mockResolvedValue(missing) }} updates={updates({ current: "v1.0.0", latest: "v1.0.0", available: false, canUpdate: false, reason: "update_unmanaged" })} />);
    expect(await screen.findByText("Unmanaged")).toBeVisible();
    for (const label of ["Commit", "Commit date", "Uncommitted changes at build time", "Executable"]) {
      expect(screen.queryByText(label)).not.toBeInTheDocument();
    }
    expect(screen.queryByText("Unknown")).not.toBeInTheDocument();
    expect(screen.getByText("Version")).toBeVisible();
    expect(await screen.findByText("No newer version is available. The running v1.0.0 is the latest.")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
    expect(screen.getByText(/Update it using the method that installed it/i)).toBeVisible();
  });

  it("keeps release notes visible after a failed check and permits another check", async () => {
    const update = updates();
    vi.mocked(update.checkUpdate).mockResolvedValue({ ...status, checkError: "upstream unavailable" });
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByText("upstream unavailable")).toBeVisible();
    expect(within(screen.getByRole("log")).getByText("Could not check GitHub for the latest release.")).toBeVisible();
    expect(screen.queryByText(/is the latest/)).not.toBeInTheDocument();
    // The reason is shown once, in the notice; the log only says when the check failed.
    expect(screen.getAllByText("upstream unavailable")).toHaveLength(1);
    expect(screen.getByText("Intermediate release")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Check for updates" })).toBeEnabled();
  });

  it("says when the sshc engine did not answer the check", async () => {
    const update = updates();
    vi.mocked(update.checkUpdate).mockRejectedValue(new Error("offline"));
    render(<AboutPage api={api} updates={update} />);
    expect(await within(await screen.findByRole("log")).findByText("Could not check for the latest release. The sshc engine did not return a result.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Check for updates" })).toBeEnabled();
  });

  it("does not log an update that had finished before the page opened", async () => {
    render(<AboutPage api={api} updates={updates({ ...status, job: { id: "old-job", target: "v1.0.0", installedVersion: "v1.0.0", state: "succeeded", problem: "" } })} />);
    expect(await screen.findByText("sshc v1.2.0 is available.")).toBeVisible();
    const lines = within(screen.getByRole("log")).getAllByRole("listitem");
    expect(lines.map((line) => line.querySelector("span")?.textContent)).toEqual([
      "Checking GitHub for the latest release…", "The latest release on GitHub is v1.2.0.",
    ]);
  });

  it("confirms the update on the page and lists its progress there instead of in the sidebar dialog", async () => {
    const update = updates();
    vi.mocked(update.previewUpdate).mockResolvedValue({ current: "v1.0.0", target: "v1.2.0", manager: "homebrew", actionToken: "confirmation" });
    vi.mocked(update.startUpdate).mockResolvedValue({ id: "about-job", target: "v1.2.0", state: "installing", problem: "" });
    render(<><UpdateBadge api={update} showsProgressDialog={false} /><AboutPage api={api} updates={update} /></>);
    const page = screen.getByRole("region", { name: "About sshc" });
    await userEvent.click(await within(page).findByRole("button", { name: "Update sshc" }));
    const dialog = await screen.findByRole("dialog", { name: "Update sshc?" });
    expect(update.startUpdate).not.toHaveBeenCalled();
    vi.mocked(update.updateStatus).mockResolvedValue({ ...status, canUpdate: false, job: { id: "about-job", target: "v1.2.0", state: "installing", problem: "" } });
    await userEvent.click(within(dialog).getByRole("button", { name: "Update sshc" }));
    await waitFor(() => expect(update.startUpdate).toHaveBeenCalledWith("v1.2.0", "confirmation"));
    expect(await within(await within(page).findByRole("log")).findByText("Updating to v1.2.0…")).toBeVisible();
    expect(screen.queryByRole("dialog", { name: "sshc update" })).not.toBeInTheDocument();
  });

  it("shows the installer's output under the steps while the update runs", async () => {
    const update = updates({ ...status, canUpdate: false, job: { id: "job-one", target: "v1.2.0", state: "installing", problem: "" } });
    vi.mocked(update.updateLog).mockResolvedValue(updateLog("==> Upgrading aida0710/tap/sshc\n", 0, { reachedCapacity: true }));
    render(<AboutPage api={api} updates={update} />);
    const output = await screen.findByRole("log", { name: "Update output" });
    expect(output).toHaveTextContent("==> Upgrading aida0710/tap/sshc");
    expect(screen.getByText("The output reached the 256 KiB limit. Anything written after that was not recorded.")).toBeVisible();
  });

  it("still shows the output of an update that ended, as after reloading once sshc restarted", async () => {
    const update = updates({ ...status, job: { id: "job-one", target: "v1.2.0", installedVersion: "v1.2.0", state: "succeeded", problem: "" } });
    vi.mocked(update.updateLog).mockResolvedValue(updateLog("sshc: starting the engine on port 8022\n"));
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByRole("log", { name: "Update output" })).toHaveTextContent("sshc: starting the engine on port 8022");
    // The finished update is not logged as progress; only the check made on opening is.
    const progress = await screen.findByRole("log", { name: "Check and update progress" });
    expect(within(progress).queryByText(/Updating|Updated/)).not.toBeInTheDocument();
  });

  it("offers the reload that the sidebar dialog would have offered while sshc restarts", async () => {
    render(<AboutPage api={api} updates={updates({ ...status, canUpdate: false, job: { id: "job", target: "v1.2.0", installedVersion: "v1.2.0", state: "restarting", problem: "" } })} />);
    expect(await screen.findByRole("button", { name: "Reload and check result" })).toBeVisible();
  });
});
