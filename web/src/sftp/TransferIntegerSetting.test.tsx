import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TransferIntegerSetting } from "./TransferIntegerSetting";

describe.each([
  { isComposing: true },
  { isComposing: false, keyCode: 229 },
])("transfer settings while the IME reports %j", (composition) => {
  it("keeps the draft and focus during composition and commits on the next Enter", () => {
    const onCommit = vi.fn();
    render(<TransferIntegerSetting label="Parallel transfers" value={2} min={1} max={8} widthClass="w-20" onCommit={onCommit} />);
    const input = screen.getByRole("spinbutton");
    input.focus();
    fireEvent.change(input, { target: { value: "4" } });

    for (const key of ["Enter", "Escape"]) {
      expect(fireEvent.keyDown(input, { key, ...composition })).toBe(true);
      expect(input).toHaveFocus();
      expect(input).toHaveValue(4);
    }
    expect(onCommit).not.toHaveBeenCalled();

    fireEvent.keyDown(input, { key: "Enter" });
    expect(input).not.toHaveFocus();
    expect(onCommit).toHaveBeenCalledExactlyOnceWith(4);
  });
});
