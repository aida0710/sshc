import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { TerminalComposer } from "./TerminalComposer";

let phone = true;

beforeEach(() => {
  phone = true;
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    get matches() { return query === mobileViewportQuery && phone; },
    media: query,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
});

function renderComposer({ value = "", disabled = false, enterSends = false } = {}) {
  const onSubmit = vi.fn();
  const onChange = vi.fn();
  render(<TerminalComposer value={value} disabled={disabled} enterSends={enterSends} onChange={onChange} onSubmit={onSubmit} />);
  return { onSubmit, onChange, field: screen.queryByRole("textbox", { name: "Text to send to the terminal" }) };
}

describe("TerminalComposer", () => {
  it("is not shown on a desktop width", () => {
    phone = false;

    const { field } = renderComposer();

    expect(field).toBeNull();
  });

  it("starts a new line on Enter and sends with the button", async () => {
    const { onSubmit, field } = renderComposer({ value: "続けて" });

    expect(field).toHaveAttribute("enterkeyhint", "enter");
    fireEvent.keyDown(field!, { key: "Enter" });
    expect(onSubmit).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("sends on Ctrl+Enter or Command+Enter from a hardware keyboard while Enter starts a new line", () => {
    const { onSubmit, field } = renderComposer({ value: "続けて" });

    fireEvent.keyDown(field!, { key: "Enter", ctrlKey: true });
    fireEvent.keyDown(field!, { key: "Enter", metaKey: true });

    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("sends on Enter when the settings choose so", () => {
    const { onSubmit, field } = renderComposer({ value: "続けて", enterSends: true });

    expect(field).toHaveAttribute("enterkeyhint", "send");
    fireEvent.keyDown(field!, { key: "Enter" });

    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("does not send the Enter that confirms a Japanese conversion", () => {
    const { onSubmit, field } = renderComposer({ value: "つづけて", enterSends: true });

    fireEvent.keyDown(field!, { key: "Enter", isComposing: true });
    fireEvent.keyDown(field!, { key: "Enter", keyCode: 229 });

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("leaves Shift+Enter to the field when Enter sends, so it can hold more than one line", () => {
    const { onSubmit, field } = renderComposer({ value: "一行目", enterSends: true });

    fireEvent.keyDown(field!, { key: "Enter", shiftKey: true });

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("cannot send while the terminal is not connected", async () => {
    const { onSubmit, field } = renderComposer({ value: "続けて", disabled: true, enterSends: true });

    expect(field).toBeDisabled();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    fireEvent.keyDown(field!, { key: "Enter" });
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(onSubmit).not.toHaveBeenCalled();
  });
});
