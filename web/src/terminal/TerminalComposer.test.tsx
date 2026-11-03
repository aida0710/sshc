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

function renderComposer({ value = "", sendable = true, enterSends = false, placeholder = undefined as string | undefined } = {}) {
  const onSubmit = vi.fn();
  const onChange = vi.fn();
  render(<TerminalComposer value={value} sendable={sendable} enterSends={enterSends} placeholder={placeholder} onChange={onChange} onSubmit={onSubmit} />);
  return { onSubmit, onChange, field: screen.queryByRole("textbox", { name: "Text to send to the terminal" }) };
}

describe("TerminalComposer", () => {
  it("says what sending does when it does not send text", () => {
    const { field } = renderComposer({ placeholder: "Send to reconnect" });
    expect(field).toHaveAttribute("placeholder", "Send to reconnect");
  });

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

  it("says that the button presses Enter while nothing is typed", async () => {
    const { onSubmit } = renderComposer();

    const button = screen.getByRole("button", { name: "Press Enter" });
    expect(button.querySelector("[data-icon]")).toHaveAttribute("data-icon", "cornerDownLeft");
    await userEvent.click(button);

    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("shows the send arrow once something is typed", () => {
    renderComposer({ value: " " });

    const button = screen.getByRole("button", { name: "Send" });
    expect(button.querySelector("[data-icon]")).toHaveAttribute("data-icon", "arrowUp");
    expect(screen.queryByRole("button", { name: "Press Enter" })).toBeNull();
  });

  it("stays editable while nothing can be sent, marks the button, and still lets a press through to say why", async () => {
    const { onSubmit, field } = renderComposer({ value: "続けて", sendable: false, enterSends: true });

    expect(field).toBeEnabled();
    const button = screen.getByRole("button", { name: "Send" });
    expect(button).toHaveAttribute("aria-disabled", "true");
    await userEvent.click(button);
    fireEvent.keyDown(field!, { key: "Enter" });

    expect(onSubmit).toHaveBeenCalledTimes(2);
  });
});
