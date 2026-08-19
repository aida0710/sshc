import type { KeyItem, KeyLocationInput, RelocateKeyResponse } from "./api";

// groupOfKeyPath は、鍵が置かれている場所からそのグループを読み取り、
// サーバーの規則を映す: keys/<group>/<file> であり、それ以外はグループに属さない。
export function groupOfKeyPath(relativePath: string): string {
  const segments = relativePath.split("/");
  if (segments.length < 3 || segments[0] !== "keys") return "";
  return segments.slice(1, -1).join("/");
}

// Folder は左の一覧に並ぶもの。**「すべて」と「グループなし」は別物である** ——
// 前者は絞り込みを外すことで、後者は ~/.ssh の直下という実在の置き場である。
// 空文字で両方を表そうとすると、どちらの意味で渡されたのかが呼び出し側から
// 読めなくなる。
export type Folder = { kind: "all" } | { kind: "ungrouped" } | { kind: "group"; name: string };

// 移す先に「すべて」は無い。絞り込みを外すことは、置き場ではない。
export type MoveTarget = Exclude<Folder, { kind: "all" }>;

export type FolderRow = { folder: Folder; count: number; depth: number };

export function sameFolder(left: Folder, right: Folder): boolean {
  if (left.kind !== right.kind) return false;
  if (left.kind === "group" && right.kind === "group") return left.name === right.name;
  return true;
}

export function itemsInFolder(items: KeyItem[], folder: Folder): KeyItem[] {
  if (folder.kind === "all") return items;
  const wanted = folder.kind === "ungrouped" ? "" : folder.name;
  return items.filter((item) => groupOfKeyPath(item.relativePath) === wanted);
}

// folderRows は左の一覧を組む。
//
// **グループは渡されたものだけである。** ディレクトリがグループなのは
// ~/.ssh/config の行がそう言っているからであり、そこに鍵が置かれているから
// ではない。ここで道から推測すると、画面が設定エンジンと違うことを言い始める
// ——宣言の無いディレクトリへ「移せる」と見せてしまえば、その移動は拒否される。
//
// **数えるのは直接入っているものだけ。** 子を親に足し込むと、work が 4 と
// 言っているのに開けると空、という画面になる。利用者が探しているのは中身で
// あって合計ではない。
export function folderRows(items: KeyItem[], groups: string[]): FolderRow[] {
  const directCount = new Map<string, number>();
  for (const item of items) {
    const group = groupOfKeyPath(item.relativePath);
    directCount.set(group, (directCount.get(group) ?? 0) + 1);
  }
  // 宣言されていないグループに入っている鍵は、どのフォルダにも数えられない。
  // 「グループなし」に足すと、置き場を偽ることになる。
  // **並びは名前順である。** グループ画面は利用者の付けた表示順で並べるが、
  // ここへ届くのは名前だけで、順序はシェルの境界で落ちている。名前順でも
  // 親は必ず子の前に来る（"a" < "a/b"）ので木としては読めるが、兄弟の
  // 並びはあちらと違いうる。揃えるなら prop を GroupMetadata に広げる話になる。
  const rows: FolderRow[] = [{ folder: { kind: "all" }, count: items.length, depth: 0 }];
  for (const name of [...groups].sort()) {
    rows.push({ folder: { kind: "group", name }, count: directCount.get(name) ?? 0, depth: name.split("/").length });
  }
  rows.push({ folder: { kind: "ungrouped" }, count: directCount.get("") ?? 0, depth: 0 });
  return rows;
}

// **この画面は「鍵」であって「~/.ssh のファイル一覧」ではない。**
//
// 分類は全ファイルに対して行われる——名前ではなく中身と権限で見るので、
// 変な名前の秘密鍵も、誰でも読めるファイルも取りこぼさない。だが分類した
// 結果を全部並べると、鍵の画面に .DS_Store と設定ファイルと known_hosts が
// 並ぶ。設定ファイルにも known_hosts にも、それぞれ専用の画面が既にある。
//
// **危ういものだけは、鍵でなくても残す。** 全部を並べていた理由がそこに
// あったので、絞り込みでその目まで潰さない。
const keyKinds = new Set(["private_key", "public_key", "certificate"]);

export type ListFilter = "keys" | "all";

export function shownItems(items: KeyItem[], filter: ListFilter): KeyItem[] {
  if (filter === "all") return items;
  return items.filter((item) => keyKinds.has(item.kind) || item.permissionRisk);
}

export type MoveOutcome = {
  moved: string[];
  unchanged: string[];
  blocked: { path: string; blockers: string[] }[];
  failed: string[];
};

type Relocate = (keyId: string, change: KeyLocationInput) => Promise<RelocateKeyResponse>;

// moveInto は選ばれた鍵をひとつの置き場へ移す。
//
// **一本が断られても、そこで止めない。** relocate は鍵ごとの取引で、解決
// できない記述や、Include のグロブが設定として読んでしまう行き先があれば、
// その鍵を拒否する。拒否をまとめて扱うと、10 本のうち 1 本のせいで 9 本が
// 動かない。動かせたものは動かし、動かせなかったものは理由と一緒に名前を出す。
//
// **黙って成功に数えない。** 断られたものと落ちたものは別々に返る。前者には
// サーバーが理由を付けており、後者には無い——その違いは利用者が次に何を
// すべきかを変える。
export async function moveInto(relocate: Relocate, items: KeyItem[], target: MoveTarget): Promise<MoveOutcome> {
  const group = target.kind === "ungrouped" ? "" : target.name;
  const outcome: MoveOutcome = { moved: [], unchanged: [], blocked: [], failed: [] };

  for (const item of items) {
    const path = item.relativePath;
    // 既にそこにあるものは触らない。書き換える config が無いのに取引を
    // 起こす理由が無く、履歴に何も起きなかった行が並ぶだけである。
    if (groupOfKeyPath(path) === group) {
      outcome.unchanged.push(path);
      continue;
    }
    try {
      const response = await relocate(item.id, { group });
      if (response.blockers.length > 0) {
        outcome.blocked.push({ path, blockers: response.blockers });
        continue;
      }
      outcome.moved.push(path);
    } catch {
      outcome.failed.push(path);
    }
  }
  return outcome;
}
