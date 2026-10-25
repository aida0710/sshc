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

function renderComposer({ value = "", disabled = false } = {}) {
  const onSubmit = vi.fn();
  const onChange = vi.fn();
  render(<TerminalComposer value={value} disabled={disabled} onChange={onChange} onSubmit={onSubmit} />);
  return { onSubmit, onChange, field: screen.queryByRole("textbox", { name: "Text to send to the terminal" }) };
}

describe("TerminalComposer", () => {
  it("is not shown on a desktop width", () => {
    phone = false;

    const { field } = renderComposer();

    expect(field).toBeNull();
  });

  it("sends with the button and with Enter", async () => {
    const { onSubmit, field } = renderComposer({ value: "続けて" });

    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    fireEvent.keyDown(field!, { key: "Enter" });

    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("does not send the Enter that confirms a Japanese conversion", () => {
    const { onSubmit, field } = renderComposer({ value: "つづけて" });

    fireEvent.keyDown(field!, { key: "Enter", isComposing: true });
    fireEvent.keyDown(field!, { key: "Enter", keyCode: 229 });

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("leaves Shift+Enter to the field so it can hold more than one line", () => {
    const { onSubmit, field } = renderComposer({ value: "一行目" });

    fireEvent.keyDown(field!, { key: "Enter", shiftKey: true });

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("cannot send while the terminal is not connected", async () => {
    const { onSubmit, field } = renderComposer({ value: "続けて", disabled: true });

    expect(field).toBeDisabled();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    fireEvent.keyDown(field!, { key: "Enter" });
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(onSubmit).not.toHaveBeenCalled();
  });
});
