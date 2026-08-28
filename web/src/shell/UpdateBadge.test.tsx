import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { UpdateBadge } from "./UpdateBadge";
import type { IntegrationsApi, UpdateStatus } from "../api/integrations";

function buildApi(status: UpdateStatus, overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return { updateStatus: vi.fn().mockResolvedValue(status), ...overrides } as unknown as IntegrationsApi;
}

describe("UpdateBadge", () => {
  it("shows the version and offers nothing when there is nothing newer", async () => {
    render(<UpdateBadge api={buildApi({ current: "0.1.0", available: false })} />);

    const version = await screen.findByText("Version 0.1.0");
    expect(version).toBeInTheDocument();
    expect(version.parentElement).toHaveClass("border-t");
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
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
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("keeps the engine version visible when the remote check cannot run", async () => {
    const api = buildApi({ current: "0.1.0", available: false }, {
      updateStatus: vi.fn().mockRejectedValue(new Error("update_check_failed")),
    });
    render(<UpdateBadge api={api} current="0.1.0" />);
    await waitFor(() => expect(api.updateStatus).toHaveBeenCalled());
    expect(screen.getByText("Version 0.1.0")).toBeVisible();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
