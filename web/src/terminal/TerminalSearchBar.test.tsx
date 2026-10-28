import { act, fireEvent, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TerminalSearchBar } from "./TerminalSearchBar";
import { useTerminalSearch } from "./useTerminalSearch";

describe.each([
  { isComposing: true },
  { isComposing: false, keyCode: 229 },
])("terminal search while the IME reports %j", (composition) => {
  it("waits for composition to end before stepping through matches or closing", () => {
    const { result } = renderHook(() => useTerminalSearch({ shortcutActive: false, region: { current: null }, returnFocus: vi.fn() }));
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

describe("terminal search focus", () => {
  function SearchHarness({ returnFocus }: { returnFocus: () => void }) {
    const search = useTerminalSearch({ shortcutActive: false, region: { current: null }, returnFocus });
    return (
      <>
        <button type="button" onClick={() => search.setQuery("")}>elsewhere</button>
        <button type="button" onClick={search.toggle}>find</button>
        {search.open ? <TerminalSearchBar search={search} mobile={false} /> : null}
      </>
    );
  }

  it("keeps typing in the query after pressing an option or a step button", async () => {
    const user = userEvent.setup();
    render(<SearchHarness returnFocus={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "find" }));
    const input = screen.getByRole("textbox", { name: "Search terminal output" });
    await user.type(input, "err");

    for (const name of ["Match case", "Use regular expression", "Previous match", "Next match"]) {
      await user.click(screen.getByRole("button", { name }));
      expect(input).toHaveFocus();
    }
    await user.keyboard("or");
    expect(input).toHaveValue("error");
  });

  it("gives focus back to the terminal when the bar closes while it has focus", async () => {
    const user = userEvent.setup();
    const returnFocus = vi.fn();
    render(<SearchHarness returnFocus={returnFocus} />);
    await user.click(screen.getByRole("button", { name: "find" }));
    await user.keyboard("{Escape}");
    expect(returnFocus).toHaveBeenCalledOnce();

    await user.click(screen.getByRole("button", { name: "find" }));
    await user.click(screen.getByRole("button", { name: "Close search" }));
    expect(returnFocus).toHaveBeenCalledTimes(2);

    // Closing from the toolbar leaves focus where it was.
    await user.click(screen.getByRole("button", { name: "find" }));
    await user.click(screen.getByRole("button", { name: "find" }));
    expect(returnFocus).toHaveBeenCalledTimes(2);
  });

  it("starts the next search, in any session, from the last query and options", () => {
    const first = renderHook(() => useTerminalSearch({ shortcutActive: false, region: { current: null }, returnFocus: vi.fn() }));
    act(() => {
      first.result.current.setQuery("deploy failed");
      first.result.current.setCaseSensitive(true);
      first.result.current.setRegex(false);
    });
    first.unmount();

    const next = renderHook(() => useTerminalSearch({ shortcutActive: false, region: { current: null }, returnFocus: vi.fn() }));
    expect(next.result.current.query).toBe("deploy failed");
    expect(next.result.current.caseSensitive).toBe(true);
    expect(next.result.current.regex).toBe(false);
  });
});
