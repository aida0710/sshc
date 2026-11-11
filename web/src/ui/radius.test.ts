import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// index.css の @theme にある角丸の名前。
const scale = ["sm", "md", "inset", "lg", "card", "composer"];
// 図の細い棒や升目に付ける 3px 以下の角丸は、部品の角丸の段階に入れない。
const hairline = String.raw`\[[123]px\]`;
const corner = "(?:-(?:t|b|l|r|s|e|tl|tr|bl|br|ss|se|es|ee))?";
const roundedClass = new RegExp(String.raw`(?<![\w-])rounded(?:-[\w.[\]%()-]+)?(?![\w-])`, "g");
const allowedClass = new RegExp(String.raw`^rounded${corner}-(?:none|full|${scale.join("|")}|${hairline})$`);

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx?$/.test(path) && !/\.test\.tsx?$/.test(path) ? [path] : [];
  });
}

// クラス名は文字列の中にしか現れない。コメントと、文字列を持たない行（変数名など）は見ない。
function mayHoldClassNames(line: string): boolean {
  const trimmed = line.trimStart();
  if (trimmed.startsWith("//") || trimmed.startsWith("*") || trimmed.startsWith("/*")) return false;
  return /["`]/.test(line);
}

describe("the corner radius", () => {
  it("always comes from the scale in index.css", () => {
    const offenders: string[] = [];
    for (const path of sources(join(__dirname, ".."))) {
      readFileSync(path, "utf8")
        .split("\n")
        .forEach((line, index) => {
          if (!mayHoldClassNames(line)) return;
          for (const found of line.match(roundedClass) ?? []) {
            if (allowedClass.test(found)) continue;
            offenders.push(`${path.slice(path.indexOf("/src/") + 1)}:${index + 1} ${found}`);
          }
        });
    }

    expect(offenders, `use a radius named in index.css instead:\n${offenders.join("\n")}`).toEqual([]);
  });
});
