import type { FileNode } from "../api/config";

export type IncludeReference = NonNullable<FileNode["includes"]>[number];

// ConfigTreeRow は、Include階層の一覧の1行である。ファイルの行の下に、そのファイルの Include の行と、
// その Include で読み込まれるファイルの行が続く。depth は字下げの段である。
export type ConfigTreeRow =
  | { kind: "file"; node: FileNode; depth: number }
  | { kind: "include"; include: IncludeReference; owner: FileNode; depth: number };

// configTreeRows は、エンジンが並べたファイルを、Include をたどった木の順に並べ直す。OpenSSH が読む順にたどり、
// 2か所から読み込まれるファイルは最初に読み込んだ Include の下に1回だけ置く（行の状態に読み込まれる回数が出る）。
// どこからも読み込まれないファイル（入口の config）が木の根になる。
export function configTreeRows(files: FileNode[]): ConfigTreeRow[] {
  const byAbsolute = new Map(files.map((node) => [node.file.absolute, node]));
  const included = new Set(
    files.flatMap((node) =>
      (node.includes ?? []).flatMap((include) => (include.matches ?? []).map((match) => match.absolute))
        .filter((absolute) => absolute !== node.file.absolute),
    ),
  );

  const rows: ConfigTreeRow[] = [];
  const placed = new Set<string>();
  function place(node: FileNode, depth: number) {
    placed.add(node.file.absolute);
    rows.push({ kind: "file", node, depth });
    for (const include of node.includes ?? []) {
      rows.push({ kind: "include", include, owner: node, depth });
      for (const match of include.matches ?? []) {
        const child = byAbsolute.get(match.absolute);
        if (child !== undefined && !placed.has(match.absolute)) place(child, depth + 1);
      }
    }
  }
  for (const node of files) {
    if (!placed.has(node.file.absolute) && !included.has(node.file.absolute)) place(node, 0);
  }
  // 互いに読み込み合うだけのファイルには根が無いので、残りを根として並べる。
  for (const node of files) {
    if (!placed.has(node.file.absolute)) place(node, 0);
  }
  return rows;
}
