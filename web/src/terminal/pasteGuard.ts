export const terminalPastePreviewCharacters = 4_096;

export type TerminalPasteRisk = "line-break" | "control-character";

export type TerminalPasteInspection = {
  requiresConfirmation: boolean;
  risks: readonly TerminalPasteRisk[];
  lineCount: number;
  endsWithLineBreak: boolean;
  preview: string;
  previewTruncated: boolean;
};

const unsafeControlCharacter = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/u;

function countLogicalLines(text: string): number {
  if (text === "") return 0;
  let lines = 1;
  for (let index = 0; index < text.length; index += 1) {
    const character = text[index];
    if (character === "\r") {
      lines += 1;
      if (text[index + 1] === "\n") index += 1;
    } else if (character === "\n") {
      lines += 1;
    }
  }
  return lines;
}

function escapePreviewCharacter(character: string): string {
  switch (character) {
    case "\\":
      return "\\\\";
    case "\r":
      return "\\r";
    case "\n":
      return "\\n\n";
    case "\t":
      return "\\t";
    case "\u007f":
      return "\\x7f";
    default: {
      const codePoint = character.codePointAt(0) ?? 0;
      if (codePoint < 0x20) return `\\x${codePoint.toString(16).padStart(2, "0")}`;
      return character;
    }
  }
}

function makePreview(text: string, limit: number): { preview: string; truncated: boolean } {
  if (!Number.isSafeInteger(limit) || limit < 0) throw new RangeError("preview character limit must be a non-negative safe integer");
  const escaped: string[] = [];
  let characters = 0;
  for (const character of text) {
    if (characters === limit) return { preview: escaped.join(""), truncated: true };
    escaped.push(escapePreviewCharacter(character));
    characters += 1;
  }
  return { preview: escaped.join(""), truncated: false };
}

/**
 * Inspects clipboard text before any line-ending normalization or bracketed-paste
 * wrapper is applied. The result deliberately does not retain the original text.
 */
export function inspectTerminalPaste(
  text: string,
  previewCharacters: number = terminalPastePreviewCharacters,
): TerminalPasteInspection {
  const risks: TerminalPasteRisk[] = [];
  if (text.includes("\r") || text.includes("\n")) risks.push("line-break");
  if (unsafeControlCharacter.test(text)) risks.push("control-character");
  const { preview, truncated } = makePreview(text, previewCharacters);
  return {
    requiresConfirmation: risks.length > 0,
    risks,
    lineCount: countLogicalLines(text),
    endsWithLineBreak: text.endsWith("\r") || text.endsWith("\n"),
    preview,
    previewTruncated: truncated,
  };
}

/** Removes exactly one terminal line ending while preserving the rest verbatim. */
export function removeFinalTerminalLineBreak(text: string): string {
  if (text.endsWith("\r\n")) return text.slice(0, -2);
  if (text.endsWith("\r") || text.endsWith("\n")) return text.slice(0, -1);
  return text;
}
