import { act, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { UpdateBadge } from "./UpdateBadge";
import type { UpdateStatus } from "../api/update";
import type { UpdateApi } from "../api/update";

function buildApi(status: UpdateStatus, overrides: Partial<UpdateApi> = {}): UpdateApi {
  return { checkUpdate: vi.fn(), updateStatus: vi.fn().mockResolvedValue(status), previewUpdate: vi.fn(), startUpdate: vi.fn(), ...overrides };
}

describe("UpdateBadge", () => {
  it("loads update information after the browser session becomes ready", async () => {
    const api = buildApi({ current: "v1.0.0", latest: "v1.1.0", available: true, canUpdate: true });
    const { rerender } = render(<UpdateBadge api={api} current="v1.0.0" enabled={false} />);
    expect(api.updateStatus).not.toHaveBeenCalled();
    rerender(<UpdateBadge api={api} current="v1.0.0" enabled />);
    expect(await screen.findByRole("button", { name: "Update sshc" })).toBeVisible();
    expect(api.updateStatus).toHaveBeenCalledTimes(1);
  });
  it("shows the version and offers nothing when there is nothing newer", async () => {
    render(<UpdateBadge api={buildApi({ current: "0.1.0", available: false })} />);

    const version = await screen.findByText("Version 0.1.0");
    expect(version).toBeInTheDocument();
    expect(version.parentElement?.parentElement).toHaveClass("border-t");
    expect(screen.getByRole("link", { name: "Version 0.1.0" })).toHaveAttribute("href", "/about");
  });

  it("links to the release rather than offering to install it", async () => {
    render(
      <UpdateBadge
        api={buildApi({
          current: "0.1.0",
          latest: "0.2.0",
          available: true,
          pageUrl: "https://example.invalid/releases/v0.2.0",
        })}
      />,
    );

    const link = await screen.findByRole("link", { name: /0\.2\.0 is available/ });
    expect(link).toHaveAttribute("href", "https://example.invalid/releases/v0.2.0");
    expect(screen.queryByRole("button", { name: "Update sshc" })).not.toBeInTheDocument();
  });

  it("keeps the engine version visible when the remote check cannot run", async () => {
    const api = buildApi({ current: "0.1.0", available: false }, {
      updateStatus: vi.fn().mockRejectedValue(new Error("update_check_failed")),
    });
    render(<UpdateBadge api={api} current="0.1.0" />);
    await waitFor(() => expect(api.updateStatus).toHaveBeenCalled());
    expect(screen.getByText("Version 0.1.0")).toBeVisible();
    expect(screen.getByRole("link", { name: "Version 0.1.0" })).toHaveAttribute("href", "/about");
  });

  it("places an optional status indicator in the version row", async () => {
    render(
      <UpdateBadge
        api={buildApi({ current: "0.1.0", available: false })}
        indicator={<span data-testid="indicator" />}
      />,
    );

    const version = await screen.findByText("Version 0.1.0");
    expect(screen.getByTestId("indicator").parentElement).toBe(version.parentElement);
    expect(version.parentElement).toHaveClass("items-center");
  });
});

it("opens About independently from the update notice", async () => {
  const onNavigate = vi.fn((event: { preventDefault(): void }) => event.preventDefault());
  const api = buildApi({ current: "v1.0.0", latest: "v1.1.0", available: true });
  render(<UpdateBadge api={api} onNavigate={onNavigate} />);
  const link = await screen.findByRole("link", { name: "Version v1.0.0" });
  expect(link).toHaveAttribute("href", "/about");
  link.click();
  expect(onNavigate).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Toggle update notice" })).toHaveAttribute("aria-expanded", "true");
});

it("rereads cached releases hourly while the page remains open and stops after unmount", async () => {
  vi.useFakeTimers();
  try {
    const api = buildApi({ current: "v1.0.0", available: false });
    const { unmount } = render(<UpdateBadge api={api} />);
    await act(async () => {});
    vi.mocked(api.updateStatus).mockResolvedValue({ current: "v1.0.0", latest: "v1.1.0", available: true, pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.1.0" });
    await act(async () => vi.advanceTimersByTimeAsync(60 * 60 * 1000 - 1));
    expect(api.updateStatus).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(api.updateStatus).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("link", { name: /v1.1.0 is available/ })).toBeVisible();
    expect(api.checkUpdate).not.toHaveBeenCalled();
    unmount();
    await act(async () => vi.advanceTimersByTimeAsync(60 * 60 * 1000));
    expect(api.updateStatus).toHaveBeenCalledTimes(2);
  } finally {
    vi.useRealTimers();
  }
});
