import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient, ApiError } from "../api/client";
import { fakeSettingsServer } from "../testing/settingsServer";
import { SFTPSettingsSection } from "./SFTPSettingsSection";

afterEach(() => vi.restoreAllMocks());

// fileTransfersOf is a preset's transfer settings as the engine keeps them.
function fileTransfersOf(server: ReturnType<typeof fakeSettingsServer>, id: string): unknown {
  return server.document().presets.find((preset) => preset.id === id)?.fileTransfers;
}

describe("Settings > SFTP", () => {
  it("shows the preset's transfer settings and the exclusions, with the defaults for what it left out", () => {
    fakeSettingsServer({ presets: [{ id: "default", fileTransfers: { largeFileParallelism: 4 } }] });
    render(<SFTPSettingsSection presetId="default" showHeading={false} />);

    const transfers = screen.getByRole("region", { name: "Transfers" });
    const exclusions = screen.getByRole("region", { name: "Transfer exclusions" });
    expect(within(transfers).getByRole("spinbutton", { name: "Speed limit" })).toHaveValue(0);
    expect(within(transfers).getByRole("combobox", { name: "Concurrent transfers" })).toHaveValue("2");
    expect(screen.getByRole("spinbutton", { name: "Split at" })).toHaveValue(100);
    expect(screen.getByRole("spinbutton", { name: "Streams" })).toHaveValue(4);
    expect(screen.getByRole("spinbutton", { name: "Chunk" })).toHaveValue(32);
    expect(within(exclusions).getByRole("textbox", { name: "Exclusion patterns (one per line)" })).toBeVisible();
    // Finishing transfers and reconnecting are global settings, on the General page.
    expect(screen.queryByRole("combobox", { name: "Finished transfers" })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Resume automatically" })).not.toBeInTheDocument();
  });

  it("saves each change in the preset being edited, leaving the defaults out", async () => {
    const server = fakeSettingsServer({ presets: [{ id: "default" }, { id: "phone", name: "Phone" }] });
    render(<SFTPSettingsSection presetId="phone" showHeading={false} />);

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");
    await waitFor(() => expect(fileTransfersOf(server, "phone")).toEqual({ maxConcurrent: 5 }));

    const speed = screen.getByRole("spinbutton", { name: "Speed limit" });
    fireEvent.change(speed, { target: { value: "2048" } });
    fireEvent.blur(speed);
    await waitFor(() => expect(fileTransfersOf(server, "phone")).toEqual({ maxConcurrent: 5, speedLimitBytesPerSecond: 2048 * 1024 }));

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "2");
    await waitFor(() => expect(fileTransfersOf(server, "phone")).toEqual({ speedLimitBytesPerSecond: 2048 * 1024 }));
    expect(fileTransfersOf(server, "default")).toBeUndefined();
  });

  it("says when a change could not be saved", async () => {
    fakeSettingsServer();
    render(<SFTPSettingsSection presetId="default" showHeading={false} />);
    vi.mocked(apiClient.mutate).mockRejectedValue(new ApiError("disk_full", 500, null));

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");

    expect(await screen.findByText("The SFTP settings could not be saved.")).toBeVisible();
  });
});
