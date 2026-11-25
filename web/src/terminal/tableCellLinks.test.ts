import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { findBufferLinks } from "./bufferLinks";

async function terminalWithRows(rows: string[]): Promise<Terminal> {
  const terminal = new Terminal({ cols: 48, rows: 24, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(rows.join("\r\n"), resolve));
  return terminal;
}

async function targetsByRow(rows: string[]): Promise<string[][]> {
  const terminal = await terminalWithRows(rows);
  try {
    return rows.map((_, index) => findBufferLinks(terminal.buffer.active, index + 1).map((link) => link.match.target));
  } finally { terminal.dispose(); }
}

describe("links in the cells of a table", () => {
  const url = "https://example.test/docs/guides/getting-started/installation/index.html";

  // スマホの幅の Claude Code の表。URL はセルの幅で折られ、となりのセルの文字は縦の中央に出る。
  it("opens the whole URL from every row of the cell it was broken over", async () => {
    const targets = await targetsByRow([
      "  ┌──────┬────────────────────────────────┐",
      "  │ Name │              URL               │",
      "  ├──────┼────────────────────────────────┤",
      "  │      │ https://example.test/docs/guid │",
      "  │ docs │ es/getting-started/installatio │",
      "  │      │ n/index.html                   │",
      "  └──────┴────────────────────────────────┘",
    ]);
    expect(targets).toEqual([[], [], [], [url], [url], [url], []]);
  });

  it("underlines only the URL's own cells on each row, not the borders or the other cells", async () => {
    const terminal = await terminalWithRows([
      "  │      │ https://example.test/docs/guid │",
      "  │ docs │ es/getting-started/installatio │",
      "  │      │ n/index.html                   │",
    ]);
    try {
      const ranges = [1, 2, 3].map((line) => findBufferLinks(terminal.buffer.active, line).map(({ range }) => [range.start.x, range.end.x]));
      expect(ranges).toEqual([[[12, 41]], [[12, 41]], [[12, 23]]]);
    } finally { terminal.dispose(); }
  });

  // 続きが短く、URL らしい記号を含まなくても、表の1つの行の中ならつなぐ。
  it("opens the whole URL when only its last letters went to the next row of the cell", async () => {
    const whole = "https://example.test/install/index.html";
    const targets = await targetsByRow([
      "├──────┼──────────────────────────────────────┤",
      "│ docs │ https://example.test/install/index.h │",
      "│      │ tml                                  │",
      "├──────┼──────────────────────────────────────┤",
    ]);
    expect(targets).toEqual([[], [whole], [whole], []]);
  });

  it("opens the whole path broken over the rows of a cell", async () => {
    const targets = await targetsByRow([
      "│ log │ /srv/app/releases/cur │",
      "│     │ rent/log/app.log      │",
    ]);
    expect(targets).toEqual([["/srv/app/releases/current/log/app.log"], ["/srv/app/releases/current/log/app.log"]]);
  });

  // 列が1つの枠では、長い行が端まで届くだけで折り返しと見分けられない。
  it("does not join the rows of a box with a single cell", async () => {
    const targets = await targetsByRow([
      "│ /srv/app/releases/current │",
      "│ /srv/app/shared/log       │",
    ]);
    expect(targets).toEqual([["/srv/app/releases/current"], ["/srv/app/shared/log"]]);
  });

  // 横の罫線の無い表（psql など）では、罫線の桁が同じ行のまとまりが表の全部の行になる。
  it("keeps the rows of a table without lines between its rows apart, also when the longest reaches the edge", async () => {
    const targets = await targetsByRow([
      "│ a │ /srv/app/releases/current │",
      "│ b │ /srv/app/shared/log       │",
      "│ c │ docs/guide/install        │",
    ]);
    expect(targets[0]).toEqual(["/srv/app/releases/current"]);
    expect(targets[1]).toEqual(["/srv/app/shared/log"]);
  });

  it("keeps two URLs apart when the row under the longest has no name", async () => {
    const targets = await targetsByRow([
      "│ a │ https://example.test/longest │",
      "│   │ https://example.test/b       │",
    ]);
    expect(targets).toEqual([["https://example.test/longest"], ["https://example.test/b"]]);
  });

  it("keeps the URLs of two rows apart when the first does not reach the edge of its cell", async () => {
    const targets = await targetsByRow([
      "│ a │ https://example.test/first     │",
      "│ b │ https://example.test/second    │",
    ]);
    expect(targets).toEqual([["https://example.test/first"], ["https://example.test/second"]]);
  });

  it("does not glue the next row's word to a URL that only happens to end at the edge of its cell", async () => {
    const targets = await targetsByRow([
      "│ See https://example.test/docs │",
      "│ for the other systems.        │",
    ]);
    expect(targets).toEqual([["https://example.test/docs"], []]);
  });

  // 幅を中身に合わせた列では、いちばん長い行がセルの端まで届く。折り返しではない。
  it("keeps the URLs of two rows apart when the first is the longest of its column and reaches the edge", async () => {
    const targets = await targetsByRow([
      "│ a │ https://example.test/longest │",
      "│ b │ https://example.test/b       │",
    ]);
    expect(targets).toEqual([["https://example.test/longest"], ["https://example.test/b"]]);
  });

  it("does not join the cells of one row: a URL ends at its cell", async () => {
    const targets = await targetsByRow(["│ https://example.test/a │ b/c.png │"]);
    expect(targets[0]).toContain("https://example.test/a");
    expect(targets[0]).not.toContain("https://example.test/a │ b/c.png");
  });

  // tree の出力は罫線で始まるが、罫線で終わらない。表ではなく、ふつうの行として読む。
  it("reads a row that only starts with a border, as tree prints, as an ordinary row", async () => {
    const targets = await targetsByRow(["│   └── https://example.test/readme"]);
    expect(targets).toEqual([["https://example.test/readme"]]);
  });
});
