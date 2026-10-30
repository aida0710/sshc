import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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

// The engine's answer to one change, held until the test gives it.
function heldCommit() {
  let answer: (committed: boolean) => void = () => undefined;
  const onCommit = vi.fn(() => new Promise<boolean>((resolve) => { answer = resolve; }));
  return { onCommit, answer: (committed: boolean) => answer(committed) };
}

describe("the SFTP transfer settings form", () => {
  it("says a change is being saved, then that it was saved, and drops that once the form is edited again", async () => {
    const { onCommit, answer } = heldCommit();
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");

    expect(onCommit).toHaveBeenCalledWith({ maxConcurrent: 5 });
    expect(screen.getByRole("status")).toHaveTextContent("Saving…");
    answer(true);
    expect(await screen.findByText("Saved.")).toBeVisible();

    await userEvent.type(screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" }), ".git");
    expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
  });

  it("does not say a change was saved when the engine did not take it", async () => {
    const { onCommit, answer } = heldCommit();
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "3");
    answer(false);

    await waitFor(() => expect(screen.queryByText("Saving…")).not.toBeInTheDocument());
    expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
  });

  it("keeps the exclusions button waiting while the rules are saved", async () => {
    const { onCommit, answer } = heldCommit();
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    await userEvent.type(screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" }), "node_modules");
    const apply = screen.getByRole("button", { name: "Save exclusions" });
    await userEvent.click(apply);

    expect(onCommit).toHaveBeenCalledWith({ excludePatterns: ["node_modules"] });
    expect(apply).toHaveAttribute("aria-busy", "true");
    answer(true);
    await waitFor(() => expect(apply).not.toHaveAttribute("aria-busy"));
    expect(screen.getByText("Saved.")).toBeVisible();
  });

  it("saves nothing when a number is left without being changed", () => {
    const { onCommit } = heldCommit();
    render(<TransferSettingsForm settings={savedSettings} onCommit={onCommit} />);
    const chunk = screen.getByRole("spinbutton", { name: "Chunk" });
    fireEvent.focus(chunk);
    fireEvent.blur(chunk);

    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.queryByText("Saving…")).not.toBeInTheDocument();
  });
});
