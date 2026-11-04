import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { RefObject } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { TerminalComposer, type ComposerField } from "./TerminalComposer";
import { touchFirstQuery } from "./touchMode";

let phone = true;
let touchScreen = false;

beforeEach(() => {
  phone = true;
  touchScreen = false;
  delete document.documentElement.dataset.keyboardOpen;
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    get matches() { return (query === mobileViewportQuery && phone) || (query === touchFirstQuery && touchScreen); },
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
  render(<TerminalComposer value={value} secret={null} sendable={sendable} enterSends={enterSends} placeholder={placeholder} onChange={onChange} onSecretChange={vi.fn()} onSubmit={onSubmit} />);
  return { onSubmit, onChange, field: screen.queryByRole("textbox", { name: "Text to send to the terminal" }) };
}

// composerAsking renders the composer with a prompt to turn on and off, as TerminalView does.
function composerAsking({ value = "", secret = "", enterSends = false }: { value?: string; secret?: string | null; enterSends?: boolean } = {}) {
  const onSubmit = vi.fn();
  const onSecretChange = vi.fn();
  const element = (answer: string | null) => (
    <TerminalComposer value={value} secret={answer} sendable enterSends={enterSends} onChange={vi.fn()} onSecretChange={onSecretChange} onSubmit={onSubmit} />
  );
  const { rerender } = render(element(secret));
  return {
    onSubmit,
    onSecretChange,
    asks: (answer: string | null) => rerender(element(answer)),
    textBox: screen.getByLabelText<HTMLTextAreaElement>("Text to send to the terminal"),
    secretField: screen.getByLabelText<HTMLInputElement>("Password to send to the terminal"),
  };
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

  describe("while the terminal asks for a password", () => {
    it("shows a password field in place of the text box, and only the text box otherwise", () => {
      const { asks, textBox, secretField } = composerAsking({ secret: null });
      expect(textBox).toBeVisible();
      expect(secretField).not.toBeVisible();

      asks("");

      expect(secretField).toBeVisible();
      expect(secretField).toHaveAttribute("type", "password");
      expect(secretField).toHaveAttribute("placeholder", "Type the password");
      expect(textBox).not.toBeVisible();
      asks(null);
      expect(textBox).toBeVisible();
      expect(secretField).not.toBeVisible();
    });

    it("keeps what was being written in the text box, out of the password field", () => {
      const { asks, textBox, secretField } = composerAsking({ value: "a long request\nto Claude Code", secret: "" });

      expect(secretField).toHaveValue("");
      expect(textBox).toHaveValue("a long request\nto Claude Code");
      asks("typed");
      expect(textBox).toHaveValue("a long request\nto Claude Code");
      expect(secretField).toHaveValue("typed");
      asks(null);
      expect(secretField).toHaveValue("");
    });

    it("moves the focus straight from one field to the other, and keeps the caret of the text box", () => {
      const { asks, textBox, secretField } = composerAsking({ value: "a long request", secret: null });
      textBox.focus();
      textBox.setSelectionRange(3, 3);
      const related: (EventTarget | null)[] = [];
      textBox.addEventListener("focusout", (event) => related.push(event.relatedTarget));

      asks("");

      expect(secretField).toHaveFocus();
      expect(related).toEqual([secretField]);
      asks(null);
      expect(textBox).toHaveFocus();
      expect(textBox.selectionStart).toBe(3);
    });

    it("keeps an open keyboard open on a touch screen, and does not raise a closed one", () => {
      touchScreen = true;
      const { asks, textBox, secretField } = composerAsking({ secret: null });
      textBox.focus();
      document.documentElement.dataset.keyboardOpen = "true";
      asks("");
      expect(secretField).toHaveFocus();

      // Back closed the keyboard and left the focus in the field.
      document.documentElement.dataset.keyboardOpen = "false";
      asks(null);
      expect(textBox).not.toHaveFocus();
    });

    it("gives no field the focus when neither had it", () => {
      const { asks, textBox, secretField } = composerAsking({ secret: null });

      asks("");
      expect(secretField).not.toHaveFocus();
      asks(null);
      expect(textBox).not.toHaveFocus();
    });

    it("sends on Enter even when Enter starts a new line in the text box", async () => {
      const { onSubmit, secretField } = composerAsking({ enterSends: false, secret: "typed" });
      expect(secretField).toHaveAttribute("enterkeyhint", "send");

      await userEvent.type(secretField, "{Enter}");

      expect(onSubmit).toHaveBeenCalledTimes(1);
    });

    it("reports what is typed as the answer", async () => {
      const { onSecretChange, secretField } = composerAsking();

      await userEvent.type(secretField, "x");

      expect(onSecretChange).toHaveBeenCalledWith("x");
    });

    it("presses Enter for an empty answer and sends a typed one, and adds no button", () => {
      const { asks } = composerAsking({ value: "a long request" });
      expect(screen.getByRole("button", { name: "Press Enter" })).toBeInTheDocument();

      asks("typed");

      expect(screen.getByRole("button", { name: "Send" })).toBeInTheDocument();
      expect(screen.getAllByRole("button")).toHaveLength(1);
    });

    it("gives fieldRef the field that takes the typing, and nothing once it is gone", () => {
      const fieldRef: RefObject<ComposerField | null> = { current: null };
      const element = (secret: string | null) => (
        <TerminalComposer value="" secret={secret} sendable enterSends={false} fieldRef={fieldRef} onChange={vi.fn()} onSecretChange={vi.fn()} onSubmit={vi.fn()} />
      );
      const { rerender, unmount } = render(element(null));
      expect(fieldRef.current).toBe(screen.getByLabelText("Text to send to the terminal"));

      rerender(element(""));
      expect(fieldRef.current).toBe(screen.getByLabelText("Password to send to the terminal"));

      unmount();
      expect(fieldRef.current).toBeNull();
    });
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
