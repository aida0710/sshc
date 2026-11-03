import { describe, expect, it } from "vitest";
import { keyBarInput, withModifiers, type KeyModifiers } from "./keySequences";

const none: KeyModifiers = { ctrl: false, alt: false };
const ctrl: KeyModifiers = { ctrl: true, alt: false };
const alt: KeyModifiers = { ctrl: false, alt: true };
const normalCursor = { applicationCursorKeys: false };
const applicationCursor = { applicationCursorKeys: true };

describe("keyBarInput for the named keys", () => {
  it("sends the sequences xterm sends for the same keys", () => {
    expect(keyBarInput({ key: "Escape" }, none, normalCursor)).toBe("\x1b");
    expect(keyBarInput({ key: "Tab" }, none, normalCursor)).toBe("\t");
    expect(keyBarInput({ key: "Shift+Tab" }, none, normalCursor)).toBe("\x1b[Z");
    expect(keyBarInput({ key: "Enter" }, none, normalCursor)).toBe("\r");
    expect(keyBarInput({ key: "Backspace" }, none, normalCursor)).toBe("\x7f");
    expect(keyBarInput({ key: "ArrowUp" }, none, normalCursor)).toBe("\x1b[A");
    expect(keyBarInput({ key: "ArrowLeft" }, none, normalCursor)).toBe("\x1b[D");
    expect(keyBarInput({ key: "Home" }, none, normalCursor)).toBe("\x1b[H");
    expect(keyBarInput({ key: "End" }, none, normalCursor)).toBe("\x1b[F");
    expect(keyBarInput({ key: "PageUp" }, none, normalCursor)).toBe("\x1b[5~");
    expect(keyBarInput({ key: "Delete" }, none, normalCursor)).toBe("\x1b[3~");
    expect(keyBarInput({ key: "F1" }, none, normalCursor)).toBe("\x1bOP");
    expect(keyBarInput({ key: "F12" }, none, normalCursor)).toBe("\x1b[24~");
    expect(keyBarInput({ key: "Ctrl+C" }, none, normalCursor)).toBe("\x03");
    expect(keyBarInput({ key: "Ctrl+R" }, none, normalCursor)).toBe("\x12");
  });

  it("sends the cursor keys, Home and End as SS3 where the program turned on application cursor keys (vim, less)", () => {
    expect(keyBarInput({ key: "ArrowUp" }, none, applicationCursor)).toBe("\x1bOA");
    expect(keyBarInput({ key: "Home" }, none, applicationCursor)).toBe("\x1bOH");
    expect(keyBarInput({ key: "PageDown" }, none, applicationCursor)).toBe("\x1b[6~");
  });

  it("adds a held Ctrl or Alt as xterm's modifier number", () => {
    expect(keyBarInput({ key: "ArrowLeft" }, ctrl, normalCursor)).toBe("\x1b[1;5D");
    expect(keyBarInput({ key: "ArrowLeft" }, alt, applicationCursor)).toBe("\x1b[1;3D");
    expect(keyBarInput({ key: "F2" }, { ctrl: true, alt: true }, normalCursor)).toBe("\x1b[1;7Q");
    expect(keyBarInput({ key: "Delete" }, ctrl, normalCursor)).toBe("\x1b[3;5~");
    expect(keyBarInput({ key: "Escape" }, alt, normalCursor)).toBe("\x1b\x1b");
    expect(keyBarInput({ key: "Backspace" }, ctrl, normalCursor)).toBe("\b");
    expect(keyBarInput({ key: "Ctrl+C" }, alt, normalCursor)).toBe("\x1b\x03");
  });

  it("sends nothing for Ctrl and Alt, which are held for the next key", () => {
    expect(keyBarInput({ key: "Control" }, none, normalCursor)).toBe("");
    expect(keyBarInput({ key: "Alt" }, none, normalCursor)).toBe("");
  });
});

describe("keyBarInput for a key the person made", () => {
  it("types its text, a line break as Enter, then Enter when it asks", () => {
    expect(keyBarInput({ label: "git st", text: "git status", enter: true }, none, normalCursor)).toBe("git status\r");
    expect(keyBarInput({ label: "two", text: "cd ~/src\nls" }, none, normalCursor)).toBe("cd ~/src\rls");
  });

  it("applies Ctrl and Alt to a key of one character, as to a typed key", () => {
    expect(keyBarInput({ label: "|", text: "|" }, alt, normalCursor)).toBe("\x1b|");
    expect(keyBarInput({ label: "c", text: "c" }, ctrl, normalCursor)).toBe("\x03");
    expect(keyBarInput({ label: "ls", text: "ls" }, ctrl, normalCursor)).toBe("ls");
  });
});

describe("withModifiers for typed text", () => {
  it("folds Ctrl into a single letter and puts ESC before it for Alt", () => {
    expect(withModifiers("c", ctrl)).toBe("\x03");
    expect(withModifiers("C", ctrl)).toBe("\x03");
    expect(withModifiers("b", alt)).toBe("\x1bb");
    expect(withModifiers("c", { ctrl: true, alt: true })).toBe("\x1b\x03");
    expect(withModifiers("|", ctrl)).toBe("|");
  });

  it("leaves a run of text alone, even one that spells a key name", () => {
    expect(withModifiers("cd /etc", ctrl)).toBe("cd /etc");
    expect(withModifiers("Esc", none)).toBe("Esc");
    expect(withModifiers("\x1b[A", ctrl)).toBe("\x1b[A");
  });
});
