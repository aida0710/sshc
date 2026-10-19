import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { About } from "../api/about";
import type { UpdateApi, UpdateStatus } from "../api/update";
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
  return { updateStatus: vi.fn().mockResolvedValue(initial), checkUpdate: vi.fn().mockResolvedValue(initial), previewUpdate: vi.fn(), startUpdate: vi.fn() };
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
    expect(screen.getByText("sshc v1.2.0 is available.")).toBeVisible();
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

  it("refreshes on demand and shares the result with the sidebar", async () => {
    const update = updates({ current: "v1.0.0", available: false });
    let finish: (status: UpdateStatus) => void = () => {};
    vi.mocked(update.checkUpdate).mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    render(<><UpdateBadge api={update} /><AboutPage api={api} updates={update} /></>);
    const check = await screen.findByRole("button", { name: "Check for updates" });
    await userEvent.click(check);
    expect(screen.getByRole("button", { name: "Checking…" })).toBeDisabled();
    const log = screen.getByRole("log", { name: "Check and update progress" });
    expect(within(log).getAllByRole("listitem").map((line) => line.textContent)).toEqual([expect.stringContaining("Checking GitHub for the latest release…")]);
    await act(async () => finish({ ...status, pageUrl: status.releases?.[0]?.pageUrl ?? "" }));
    expect(update.checkUpdate).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("link", { name: /v1.2.0 is available/ })).toBeVisible();
    expect(within(log).getByText("The latest release on GitHub is v1.2.0.")).toBeVisible();
    expect(check).toBeEnabled();
  });

  it("adds a line for every check, so a repeated check with the same result is still visible", async () => {
    const update = updates({ ...status, latest: "v1.0.0", available: false, canUpdate: false });
    render(<AboutPage api={api} updates={update} />);
    expect(await screen.findByText("The running v1.0.0 is the latest version.")).toBeVisible();
    expect(screen.queryByRole("log")).not.toBeInTheDocument();
    const check = screen.getByRole("button", { name: "Check for updates" });
    await userEvent.click(check);
    await waitFor(() => expect(check).toBeEnabled());
    await userEvent.click(check);
    await waitFor(() => expect(check).toBeEnabled());
    const lines = within(screen.getByRole("log")).getAllByRole("listitem");
    expect(lines.map((line) => line.querySelector("span")?.textContent)).toEqual([
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
    expect(screen.getByText("The running v1.0.0 is the latest version.")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
    expect(screen.getByText(/Update it using the method that installed it/i)).toBeVisible();
  });

  it("keeps release notes visible after a failed check and permits another check", async () => {
    const update = updates();
    vi.mocked(update.checkUpdate).mockResolvedValue({ ...status, checkError: "upstream unavailable" });
    render(<AboutPage api={api} updates={update} />);
    await userEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
    expect(await screen.findByText("upstream unavailable")).toBeVisible();
    expect(within(screen.getByRole("log")).getByText("Could not check GitHub for the latest release.")).toBeVisible();
    // The reason is shown once, in the notice; the log only says when the check failed.
    expect(screen.getAllByText("upstream unavailable")).toHaveLength(1);
    expect(screen.getByText("Intermediate release")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Check for updates" })).toBeEnabled();
  });

  it("says when the sshc engine did not answer the check", async () => {
    const update = updates();
    vi.mocked(update.checkUpdate).mockRejectedValue(new Error("offline"));
    render(<AboutPage api={api} updates={update} />);
    await userEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
    expect(await within(screen.getByRole("log")).findByText("Could not check for the latest release. The sshc engine did not return a result.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Check for updates" })).toBeEnabled();
  });

  it("does not log an update that had finished before the page opened", async () => {
    render(<AboutPage api={api} updates={updates({ ...status, job: { id: "old-job", target: "v1.0.0", installedVersion: "v1.0.0", state: "succeeded", problem: "" } })} />);
    expect(await screen.findByText("sshc v1.2.0 is available.")).toBeVisible();
    expect(screen.queryByRole("log")).not.toBeInTheDocument();
  });

  it("uses the existing update confirmation and sends progress to the sidebar dialog", async () => {
    const update = updates();
    vi.mocked(update.previewUpdate).mockResolvedValue({ current: "v1.0.0", target: "v1.2.0", manager: "homebrew", actionToken: "confirmation", actionExpiresAt: "2026-10-19T02:00:00Z" });
    vi.mocked(update.startUpdate).mockResolvedValue({ id: "about-job", target: "v1.2.0", state: "installing", problem: "" });
    render(<><UpdateBadge api={update} /><AboutPage api={api} updates={update} /></>);
    const page = screen.getByRole("region", { name: "About sshc" });
    await userEvent.click(await within(page).findByRole("button", { name: "Update sshc" }));
    const dialog = await screen.findByRole("dialog", { name: "Update sshc?" });
    expect(update.startUpdate).not.toHaveBeenCalled();
    vi.mocked(update.updateStatus).mockResolvedValue({ ...status, canUpdate: false, job: { id: "about-job", target: "v1.2.0", state: "installing", problem: "" } });
    await userEvent.click(within(dialog).getByRole("button", { name: "Update sshc" }));
    await waitFor(() => expect(update.startUpdate).toHaveBeenCalledWith("v1.2.0", "confirmation"));
    expect(await screen.findByRole("dialog", { name: "sshc update" })).toBeVisible();
    expect(within(within(page).getByRole("log")).getByText("Updating to v1.2.0…")).toBeVisible();
  });
});
