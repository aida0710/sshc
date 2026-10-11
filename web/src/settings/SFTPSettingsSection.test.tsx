import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TransferSettings } from "../sftp/api";
import { SFTPSettingsSection } from "./SFTPSettingsSection";

const savedSettings: TransferSettings = {
  maxConcurrent: 2,
  clearCompletedAfterSeconds: 300,
  processingStopped: false,
  largeFileThresholdBytes: 100 << 20,
  largeFileParallelism: 4,
  largeFileChunkBytes: 32 << 20,
  speedLimitBytesPerSecond: 0,
  autoReconnect: false,
  maxReconnectAttempts: 0,
  excludePatterns: [],
};

const manager = vi.hoisted(() => {
  // useSyncExternalStore needs the same snapshot until something changes.
  const noJobs: unknown[] = [];
  return {
    subscribe: () => () => undefined,
    getSnapshot: () => noJobs,
    getSettings: vi.fn(),
    reconcile: vi.fn(),
    applySettings: vi.fn(),
  };
});

vi.mock("../sftp/transferManager", () => ({ sftpTransferManager: manager }));

describe("Settings > SFTP", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    manager.getSettings.mockReturnValue(savedSettings);
    manager.reconcile.mockResolvedValue(undefined);
    manager.applySettings.mockResolvedValue(undefined);
  });

  it("shows the transfer settings in groups once the engine's queue is read", async () => {
    render(<SFTPSettingsSection showHeading={false} />);
    expect(await screen.findByRole("group", { name: "Speed and recovery" })).toBeVisible();
    expect(screen.getByRole("group", { name: "Queue and finished transfers" })).toBeVisible();
    expect(screen.getByRole("group", { name: "Split transfers" })).toBeVisible();
    expect(screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" })).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Clear finished after" })).toHaveValue("300");
    expect(screen.getByRole("spinbutton", { name: "Split at" })).toHaveValue(100);
    expect(screen.getByRole("spinbutton", { name: "Streams" })).toHaveValue(4);
    expect(screen.getByRole("spinbutton", { name: "Chunk" })).toHaveValue(32);
    expect(manager.reconcile).toHaveBeenCalledOnce();
  });

  it("sends each change to the engine on its own", async () => {
    render(<SFTPSettingsSection showHeading={false} />);
    await userEvent.selectOptions(await screen.findByRole("combobox", { name: "Concurrent transfers" }), "5");
    expect(manager.applySettings).toHaveBeenLastCalledWith({ maxConcurrent: 5 });

    const speed = screen.getByRole("spinbutton", { name: "Speed limit" });
    fireEvent.change(speed, { target: { value: "2048" } });
    fireEvent.blur(speed);
    expect(manager.applySettings).toHaveBeenLastCalledWith({ speedLimitBytesPerSecond: 2048 * 1024 });
    expect(screen.getByText("KiB/s")).toBeVisible();

    await userEvent.click(screen.getByRole("checkbox", { name: "Recover after connection loss" }));
    expect(manager.applySettings).toHaveBeenLastCalledWith({ autoReconnect: true, maxReconnectAttempts: 3 });

    const chunk = screen.getByRole("spinbutton", { name: "Chunk" });
    fireEvent.change(chunk, { target: { value: "41" } });
    fireEvent.blur(chunk);
    expect(manager.applySettings).toHaveBeenLastCalledWith({ largeFileChunkBytes: 41 << 20 });
  });

  it("keeps the form closed until the queue can be read, so defaults are never saved", async () => {
    manager.reconcile.mockRejectedValueOnce(new Error("offline"));
    render(<SFTPSettingsSection showHeading={false} />);
    expect(await screen.findByText("SFTP settings could not be loaded. They cannot be saved until they load.")).toBeVisible();
    expect(screen.queryByRole("group", { name: "Speed and recovery" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("group", { name: "Speed and recovery" })).toBeVisible();
  });

  it("says why a change did not go through and reads the queue again", async () => {
    manager.applySettings.mockRejectedValueOnce(new ApiError("sftp_transfer_state", 409, null));
    render(<SFTPSettingsSection showHeading={false} />);
    await userEvent.selectOptions(await screen.findByRole("combobox", { name: "Concurrent transfers" }), "5");
    expect(await screen.findByText("The transfer state changed. The list has been refreshed.")).toBeVisible();
    expect(manager.reconcile).toHaveBeenCalledTimes(2);
  });
});
