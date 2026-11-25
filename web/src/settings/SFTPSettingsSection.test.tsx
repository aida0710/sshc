import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient, ApiError } from "../api/client";
import { renderSettings, saveBar, saveButton, saveSettings } from "../testing/settingsDraft";
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
    renderSettings(<SFTPSettingsSection presetId="default" showHeading={false} />);

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

  it("saves the changes in the preset being edited, leaving the defaults out", async () => {
    const server = fakeSettingsServer({ presets: [{ id: "default" }, { id: "phone", name: "Phone" }] });
    renderSettings(<SFTPSettingsSection presetId="phone" showHeading={false} />);

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");
    const speed = screen.getByRole("spinbutton", { name: "Speed limit" });
    fireEvent.change(speed, { target: { value: "2048" } });
    expect(fileTransfersOf(server, "phone")).toBeUndefined();
    await saveSettings();
    expect(fileTransfersOf(server, "phone")).toEqual({ maxConcurrent: 5, speedLimitBytesPerSecond: 2048 * 1024 });

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "2");
    await userEvent.click(saveButton());
    await vi.waitFor(() => expect(fileTransfersOf(server, "phone")).toEqual({ speedLimitBytesPerSecond: 2048 * 1024 }));
    expect(fileTransfersOf(server, "default")).toBeUndefined();
  });

  it("offers to save from the first character typed, and not while a number is out of range", async () => {
    fakeSettingsServer();
    renderSettings(<SFTPSettingsSection presetId="default" showHeading={false} />);
    const streams = screen.getByRole("spinbutton", { name: "Streams" });
    expect(saveBar()).toBeNull();

    fireEvent.change(streams, { target: { value: "8" } });
    expect(saveButton()).toBeEnabled();

    fireEvent.change(streams, { target: { value: "800" } });
    expect(streams).toHaveValue(800);
    expect(streams).toHaveAccessibleDescription("Streams: enter a whole number from 1 to 128.");
    expect(saveButton()).toBeDisabled();
  });

  it("says when the changes could not be saved, and keeps them", async () => {
    fakeSettingsServer();
    renderSettings(<SFTPSettingsSection presetId="default" showHeading={false} />);
    vi.mocked(apiClient.mutate).mockRejectedValue(new ApiError("disk_full", 500, null));

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");
    await userEvent.click(saveButton());

    expect(await screen.findByText("The settings could not be saved.")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Concurrent transfers" })).toHaveValue("5");
  });

  it("turns thumbnails off in the preset being edited, and on again by leaving nothing behind", async () => {
    const server = fakeSettingsServer({ presets: [{ id: "default" }, { id: "phone", name: "Phone" }] });
    renderSettings(<SFTPSettingsSection presetId="phone" showHeading={false} />);
    const interfaceOf = (id: string) => server.document().presets.find((preset) => preset.id === id)?.interface;

    const thumbnails = screen.getByRole("checkbox", { name: "Show thumbnails of images" });
    expect(thumbnails).toBeChecked();
    await userEvent.click(thumbnails);
    await saveSettings();
    expect(interfaceOf("phone")).toEqual({ hideSFTPThumbnails: true });
    expect(interfaceOf("default")).toBeUndefined();

    await userEvent.click(screen.getByRole("checkbox", { name: "Show thumbnails of images" }));
    await userEvent.click(saveButton());
    await vi.waitFor(() => expect(interfaceOf("phone")).toBeUndefined());
  });
});
