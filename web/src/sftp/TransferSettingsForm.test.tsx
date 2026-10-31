import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TransferSettingsForm, type PresetTransferSettings } from "./TransferSettingsForm";

const savedSettings: PresetTransferSettings = {
  maxConcurrent: 2,
  largeFileThresholdBytes: 100 << 20,
  largeFileParallelism: 4,
  largeFileChunkBytes: 32 << 20,
  speedLimitBytesPerSecond: 0,
  excludePatterns: [],
};

describe("the SFTP transfer settings form", () => {
  it("saves a choice as soon as it is made", async () => {
    const onCommit = vi.fn().mockResolvedValue(true);
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");

    expect(onCommit).toHaveBeenCalledWith({ maxConcurrent: 5 });
  });

  it("saves the exclusion rules when their box is left, not while they are typed", async () => {
    const onCommit = vi.fn().mockResolvedValue(true);
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    await userEvent.type(screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" }), "node_modules");
    expect(onCommit).not.toHaveBeenCalled();

    await userEvent.tab();
    expect(onCommit).toHaveBeenCalledWith({ excludePatterns: ["node_modules"] });
  });

  it("saves nothing when a number is left without being changed", () => {
    const onCommit = vi.fn().mockResolvedValue(true);
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    const chunk = screen.getByRole("spinbutton", { name: "Chunk" });
    fireEvent.focus(chunk);
    fireEvent.blur(chunk);

    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.queryByText("Saving…")).not.toBeInTheDocument();
  });
});
