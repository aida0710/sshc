import { describe, expect, it } from "vitest";
import { scrollbackFileName, scrollbackText } from "./scrollbackFile";

// A buffer of rows as xterm keeps them; a row that continues the one above is wrapped.
function buffer(rows: { text: string; wrapped?: boolean }[]) {
  return {
    length: rows.length,
    getLine: (index: number) => {
      const row = rows[index];
      return row === undefined ? undefined : { isWrapped: row.wrapped === true, translateToString: () => row.text };
    },
  };
}

describe("saving the scrollback to a file", () => {
  it("keeps every row, not only the last 200, joins wrapped rows and drops the empty end", () => {
    const rows: { text: string; wrapped?: boolean }[] = Array.from({ length: 500 }, (_, index) => ({ text: `line ${index}` }));
    rows.push({ text: "a long line that" }, { text: " wrapped", wrapped: true }, { text: "" }, { text: "" });

    const text = scrollbackText(buffer(rows));

    expect(text.split("\n")).toHaveLength(501);
    expect(text.startsWith("line 0\nline 1\n")).toBe(true);
    expect(text.endsWith("line 499\na long line that wrapped")).toBe(true);
  });

  it("names the file after the pane and the local time, without characters a file name cannot hold", () => {
    const savedAt = new Date(2026, 9, 28, 9, 5, 7);

    expect(scrollbackFileName("osaka: ~/src/app", savedAt)).toBe("osaka_ ~_src_app-20261028-090507.txt");
    expect(scrollbackFileName("  ", savedAt)).toBe("terminal-20261028-090507.txt");
    expect(scrollbackFileName("x".repeat(200), savedAt)).toBe(`${"x".repeat(80)}-20261028-090507.txt`);
  });
});
