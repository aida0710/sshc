import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TerminalPresetBar } from "./TerminalPresetBar";

const presets = [{ id: "phone", name: "Phone", settings: { fontSize: 12 } }];

function renderBar(overrides: Partial<Parameters<typeof TerminalPresetBar>[0]> = {}) {
  const props = {
    presets, editing: "default", inUse: "default", disabled: false,
    onOpen: vi.fn(), onUseHere: vi.fn(), onCreate: vi.fn(async () => undefined), onRename: vi.fn(async () => undefined), onDelete: vi.fn(async () => undefined),
    ...overrides,
  };
  render(<TerminalPresetBar {...props} />);
  return props;
}

describe("TerminalPresetBar", () => {
  it("marks the set this browser uses, which for the standard set can be neither renamed nor deleted", () => {
    renderBar({ editing: "default", inUse: "default" });
    expect(screen.getByText("In use in this browser")).toBeVisible();
    expect(screen.getByRole("button", { name: "Rename" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
  });

  it("offers to use the open set in this browser when it uses another", async () => {
    const props = renderBar({ editing: "phone", inUse: "default" });
    await userEvent.click(screen.getByRole("button", { name: "Use in this browser" }));
    expect(props.onUseHere).toHaveBeenCalled();
  });

  it("names a new set from the one that is open", async () => {
    const props = renderBar({ editing: "phone" });
    await userEvent.click(screen.getByRole("button", { name: "New preset" }));
    expect(screen.getByRole("dialog", { name: "New preset from Phone" })).toBeVisible();
    await userEvent.type(screen.getByRole("textbox", { name: "Preset name" }), "Talk");
    await userEvent.click(screen.getAllByRole("button", { name: "New preset" }).at(-1)!);
    expect(props.onCreate).toHaveBeenCalledWith("Talk");
  });

  it("asks before deleting a set from every machine", async () => {
    const props = renderBar({ editing: "phone" });
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.click(screen.getByRole("button", { name: "Delete everywhere" }));
    expect(props.onDelete).toHaveBeenCalled();
  });
});
