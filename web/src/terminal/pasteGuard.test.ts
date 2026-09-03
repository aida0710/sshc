import { describe, expect, it } from "vitest";
import { inspectTerminalPaste, removeFinalTerminalLineBreak } from "./pasteGuard";

describe("terminal paste guard", () => {
  it("allows ordinary one-line text without retaining it in the inspection result", () => {
    const inspection = inspectTerminalPaste("printf 'safe'");

    expect(inspection).toMatchObject({
      requiresConfirmation: false,
      risks: [],
      lineCount: 1,
      endsWithLineBreak: false,
      preview: "printf 'safe'",
      previewTruncated: false,
    });
    expect(inspection).not.toHaveProperty("text");
  });

  it.each([
    ["one\ntwo", 2, false],
    ["one\r\ntwo", 2, false],
    ["one\rtwo", 2, false],
    ["one\n", 2, true],
  ])("requires confirmation for line endings in %j", (text, lineCount, endsWithLineBreak) => {
    expect(inspectTerminalPaste(text)).toMatchObject({
      requiresConfirmation: true,
      risks: ["line-break"],
      lineCount,
      endsWithLineBreak,
    });
  });

  it("counts mixed CRLF, bare CR, and LF as three logical separators", () => {
    expect(inspectTerminalPaste("one\r\ntwo\rthree\nfour").lineCount).toBe(4);
  });

  it("requires confirmation for executable terminal control characters", () => {
    expect(inspectTerminalPaste("hello\u001b[31m\u0000\u007f")).toMatchObject({
      requiresConfirmation: true,
      risks: ["control-character"],
      lineCount: 1,
      endsWithLineBreak: false,
      preview: "hello\\x1b[31m\\x00\\x7f",
    });
  });

  it("makes line endings, tabs, and literal backslashes unambiguous in the preview", () => {
    expect(inspectTerminalPaste("one\\path\t\r\ntwo\rthree").preview).toBe(
      "one\\\\path\\t\\r\\n\ntwo\\rthree",
    );
  });

  it("bounds the preview by Unicode characters without splitting a surrogate pair", () => {
    expect(inspectTerminalPaste("😀secret", 1)).toMatchObject({
      preview: "😀",
      previewTruncated: true,
    });
    expect(inspectTerminalPaste("😀", 1).previewTruncated).toBe(false);
  });

  it("rejects invalid preview limits", () => {
    expect(() => inspectTerminalPaste("text", -1)).toThrow(RangeError);
    expect(() => inspectTerminalPaste("text", 1.5)).toThrow(RangeError);
  });

  it("reports an empty value as zero lines", () => {
    expect(inspectTerminalPaste("")).toMatchObject({
      requiresConfirmation: false,
      lineCount: 0,
      preview: "",
    });
  });

  it.each([
    ["one\r\ntwo\r\n", "one\r\ntwo"],
    ["one\ntwo\n", "one\ntwo"],
    ["one\rtwo\r", "one\rtwo"],
    ["one\n\n", "one\n"],
    ["one", "one"],
    ["", ""],
  ])("removes exactly one final terminal line ending from %j", (text, expected) => {
    expect(removeFinalTerminalLineBreak(text)).toBe(expected);
  });
});
