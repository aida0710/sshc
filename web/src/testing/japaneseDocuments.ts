import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

// webの外にある日本語（pages、README、docs/の文書、Androidの文言）をテストから読む。

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const pagesRoot = join(repositoryRoot, "pages");
const docsRoot = join(repositoryRoot, "docs");
const readme = join(repositoryRoot, "README.md");
const androidJapaneseStrings = join(repositoryRoot, "android", "app", "src", "main", "res", "values-ja", "strings.xml");

export type JapanesePage = { path: string; lines: string[] };

function markdownFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      // pages/enは英語版、.vitepressはサイトの設定なので、日本語の本文ではない。
      const skipped = entry.name === "en" || entry.name === "node_modules" || entry.name.startsWith(".");
      return skipped ? [] : markdownFiles(path);
    }
    return entry.name.endsWith(".md") ? [path] : [];
  });
}

// pathはリポジトリからの相対パスで、区切りは`/`にそろえる。
function readJapanesePage(file: string): JapanesePage {
  return {
    path: relative(repositoryRoot, file).split(sep).join("/"),
    lines: readFileSync(file, "utf8").split("\n"),
  };
}

export function japanesePages(): JapanesePage[] {
  return markdownFiles(pagesRoot).map(readJapanesePage);
}

// 名前に日付を持つdocs/の文書は、その日の監査や調査の記録である。当時の画面の文言を
// 引用しているので、今の用語に書き換えない。
const datedRecordName = /\d{4}-\d{2}-\d{2}/;
// 使わない語そのものを用語表に挙げている文書。
const termTableDocument = "writing-style.md";

export function japaneseReadme(): JapanesePage {
  return readJapanesePage(readme);
}

// READMEと、docs/直下の文書。docs/releases（公開済みのリリースノート）とdocs/superpowers
// （作業の計画）は書いた時点の記録なので入れない。
export function japaneseRepositoryDocuments(): JapanesePage[] {
  const docs = readdirSync(docsRoot, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
    .filter((entry) => !datedRecordName.test(entry.name) && entry.name !== termTableDocument)
    .map((entry) => readJapanesePage(join(docsRoot, entry.name)));
  return [japaneseReadme(), ...docs];
}

export function androidJapaneseLabels(): string[] {
  const xml = readFileSync(androidJapaneseStrings, "utf8");
  return [...xml.matchAll(/<string name="[^"]+">([^<]*)<\/string>/g)].map(([, label = ""]) =>
    label.replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("\\'", "'").replaceAll("&amp;", "&"),
  );
}
