import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderSettings } from "../testing/settingsDraft";
import { fakeSettingsServer } from "../testing/settingsServer";
import { TransferSettingsForm, type PresetTransferSettings } from "./TransferSettingsForm";

const savedSettings: PresetTransferSettings = {
  maxConcurrent: 2,
  largeFileThresholdBytes: 100 << 20,
  largeFileParallelism: 4,
  largeFileChunkBytes: 32 << 20,
  speedLimitBytesPerSecond: 0,
  excludePatterns: [],
};

beforeEach(() => { fakeSettingsServer(); });

describe("the SFTP transfer settings form", () => {
  it("hands on a choice as soon as it is made", async () => {
    const onChange = vi.fn();
    renderSettings(<TransferSettingsForm settings={savedSettings} onChange={onChange} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");

    expect(onChange).toHaveBeenCalledWith("maxConcurrent", 5);
  });

  it("hands on a number in the setting's own unit", () => {
    const onChange = vi.fn();
    renderSettings(<TransferSettingsForm settings={savedSettings} onChange={onChange} />);
    fireEvent.change(screen.getByRole("spinbutton", { name: "Chunk" }), { target: { value: "64" } });

    expect(onChange).toHaveBeenCalledWith("largeFileChunkBytes", 64 << 20);
  });

  it("hands on nothing when a number is left without being changed", () => {
    const onChange = vi.fn();
    renderSettings(<TransferSettingsForm settings={savedSettings} onChange={onChange} />);
    const chunk = screen.getByRole("spinbutton", { name: "Chunk" });
    fireEvent.focus(chunk);
    fireEvent.blur(chunk);

    expect(onChange).not.toHaveBeenCalled();
  });
});
