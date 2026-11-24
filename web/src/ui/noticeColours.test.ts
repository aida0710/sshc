import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// 知らせは、意味の色を字に使わない（利用者が選んだ、枠と地だけに色を付けるカード）。文として書く知らせは、
// Notice（枠に入れる知らせ）、NoteLine（入力欄や行の下の1行）、noticeBand（区画の上の帯）のどれかに載せる。
// この検査は、段落・見出し・一覧に失敗や注意の色の字を直に付けた所と、注意の地を手で敷いた箱を見つける。
// 状態の語や差分の行のように、色そのものが値を表す所は、直前の行に notice-colour-exempt と理由を書いて外す。

// 文を入れる要素に、失敗か注意の色の字を付けた所。
const colouredSentence = /<(?:p|h[1-6]|li|ul|ol|div|section)\b[^>]*\btext-(?:danger|notice-ink)\b/;
// 注意か失敗の地を手で敷いた箱。hover: などの、押したときだけの色は数えない。
const handMadeBox = /\brounded-(?:inset|md|lg)\b[^"`]*(?<![:\w-])bg-(?:notice|danger\/\d+)\b|(?<![:\w-])bg-(?:notice|danger\/\d+)\b[^"`]*\brounded-(?:inset|md|lg)\b/;
const exemption = "notice-colour-exempt";

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx$/.test(path) && !/\.test\.tsx$/.test(path) ? [path] : [];
  });
}

describe("notices", () => {
  it("colour only their frame and ground, through the shared parts", () => {
    const offenders: string[] = [];
    for (const path of sources(join(__dirname, ".."))) {
      const lines = readFileSync(path, "utf8").split("\n");
      lines.forEach((line, index) => {
        if (!colouredSentence.test(line) && !handMadeBox.test(line)) return;
        if (line.includes(exemption) || (lines[index - 1] ?? "").includes(exemption)) return;
        offenders.push(`${path.slice(path.indexOf("/src/") + 1)}:${index + 1}`);
      });
    }

    expect(offenders, `use Notice, NoteLine or noticeBand, or say why the colour is a value:\n${offenders.join("\n")}`).toEqual([]);
  });
});
