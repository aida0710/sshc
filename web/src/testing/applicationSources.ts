import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

// アプリのソース（src の下の .ts と .tsx。テストは除く）を、見た目の決まりの検査が走査するための読み手。

const sourceRoot = join(__dirname, "..");

// SourceLine は、ソースの1行と、報告に書く場所（src/ui/form.tsx:12 の形）である。
export type SourceLine = { where: string; text: string };

function sourceFilesUnder(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sourceFilesUnder(path);
    return /\.tsx?$/.test(path) && !/\.test\.tsx?$/.test(path) ? [path] : [];
  });
}

// SourceFile は、ソースの1ファイルと、報告に書く名前（src/ui/form.tsx の形）である。
export type SourceFile = { name: string; text: string };

export function applicationSourceFiles(): SourceFile[] {
  return sourceFilesUnder(sourceRoot).map((path) => ({ name: path.slice(path.indexOf("/src/") + 1), text: readFileSync(path, "utf8") }));
}

export function applicationSourceLines(): SourceLine[] {
  return applicationSourceFiles().flatMap((file) =>
    file.text.split("\n").map((text, index) => ({ where: `${file.name}:${index + 1}`, text })),
  );
}

// readStylesheet は、決まりの定義を持つ index.css を返す。
export function readStylesheet(): string {
  return readFileSync(join(sourceRoot, "index.css"), "utf8");
}

// クラス名は文字列の中にしか現れない。コメントと、文字列を持たない行（変数名など）は見ない。
export function mayHoldClassNames(line: string): boolean {
  const trimmed = line.trimStart();
  if (trimmed.startsWith("//") || trimmed.startsWith("*") || trimmed.startsWith("/*")) return false;
  return /["`]/.test(line);
}
