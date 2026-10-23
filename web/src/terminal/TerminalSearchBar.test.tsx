import { fireEvent, render, renderHook, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TerminalSearchBar } from "./TerminalSearchBar";
import { useTerminalSearch } from "./useTerminalSearch";

describe.each([
  { isComposing: true },
  { isComposing: false, keyCode: 229 },
])("terminal search while the IME reports %j", (composition) => {
  it("waits for composition to end before stepping through matches or closing", () => {
    const { result } = renderHook(() => useTerminalSearch({ shortcutActive: false, region: { current: null } }));
    const step = vi.fn();
    const close = vi.fn();
    render(<TerminalSearchBar search={{ ...result.current, step, close }} mobile={false} />);
    const input = screen.getByRole("textbox");

    for (const key of ["Enter", "Escape"]) {
      expect(fireEvent.keyDown(input, { key, ...composition })).toBe(true);
    }
    expect(step).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
    expect(input).toHaveFocus();

    fireEvent.keyDown(input, { key: "Enter" });
    expect(step).toHaveBeenLastCalledWith(1);
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(step).toHaveBeenLastCalledWith(-1);
    expect(step).toHaveBeenCalledTimes(2);
    fireEvent.keyDown(input, { key: "Escape" });
    expect(close).toHaveBeenCalledOnce();
  });
});
