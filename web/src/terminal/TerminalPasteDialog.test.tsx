import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TerminalPasteDialog } from "./TerminalPasteDialog";

function open(text: string) {
  const onPaste = vi.fn();
  const onCancel = vi.fn();
  render(<TerminalPasteDialog target="test" text={text} onPaste={onPaste} onCancel={onCancel} />);
  return { onPaste, onCancel, editor: screen.getByRole("textbox", { name: "Edit paste" }) };
}

describe("TerminalPasteDialog", () => {
  it("reviews edited content without sending it and removes only the final edited newline", async () => {
    const { editor, onPaste } = open("echo original\n");
    fireEvent.change(editor, { target: { value: "echo edited\necho second\n\n" } });
    expect(onPaste).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Paste preview with control characters made visible")).toHaveTextContent("echo edited");
    await userEvent.click(screen.getByRole("button", { name: "Paste without final Enter" }));
    expect(onPaste).toHaveBeenCalledExactlyOnceWith("echo edited\necho second\n");
  });

  it("updates warnings and final Enter controls as the text changes", async () => {
    const { editor, onPaste } = open("echo original\n");
    fireEvent.change(editor, { target: { value: "echo edited" } });
    expect(screen.queryByRole("button", { name: "Paste without final Enter" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Paste" }));
    expect(onPaste).toHaveBeenCalledExactlyOnceWith("echo edited");
  });

  it("preserves untouched carriage returns and control characters", async () => {
    const original = "first\r\nsecond\r\u001b";
    const { onPaste } = open(original);
    expect(screen.getByLabelText("Paste preview with control characters made visible")).toHaveTextContent("\\x1b");
    await userEvent.click(screen.getByRole("button", { name: "Paste" }));
    expect(onPaste).toHaveBeenCalledExactlyOnceWith(original);
  });

  it("edits the complete text even when the escaped preview is truncated", async () => {
    const { editor, onPaste, onCancel } = open("x".repeat(5000) + "\n");
    expect(editor).toHaveValue("x".repeat(5000) + "\n");
    fireEvent.change(editor, { target: { value: "replaced\n" } });
    await userEvent.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalled();
    expect(onPaste).not.toHaveBeenCalled();
  });
});
