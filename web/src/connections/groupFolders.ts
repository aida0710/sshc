import type { HostEntry, Notice } from "../api/config";
import type { KeyItem } from "../keys/api";
import { itemsInFolder, keyItemGroups } from "../keys/organizer";

// グループはフォルダである。接続先は connections/<グループ>/、鍵は keys/<グループ>/ に置く。
// ここでは、overview と鍵の一覧が読んだものから、その2つのフォルダの中身を並べる。

export type ConnectionFile = { name: string; aliases: string[] };

function fileName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

// connectionFilesOf は、グループの直下の接続先が書かれたファイルを、読み込む順に並べる。
export function connectionFilesOf(hosts: readonly HostEntry[], group: string): ConnectionFile[] {
  const files = new Map<string, string[]>();
  for (const host of hosts) {
    if ((host.group ?? "") !== group || host.identity.alias === "") continue;
    const path = host.file.path ?? host.file.absolute;
    files.set(path, [...(files.get(path) ?? []), host.identity.alias]);
  }
  return [...files].map(([path, aliases]) => ({ name: fileName(path), aliases }));
}

// companions は、対になる公開鍵と証明書。秘密鍵と同じ名前で始まるものは、続きの部分（.pub など）だけにする。
export type KeyFile = { name: string; companions: string[] };

// keyFilesOf は、keys/<グループ>/ の直下の鍵を並べる。公開鍵と証明書は、SSH Keys のページと同じく
// 1行にせず、対になる秘密鍵に添える。
export function keyFilesOf(items: readonly KeyItem[], group: string): KeyFile[] {
  return keyItemGroups(itemsInFolder([...items], { kind: "group", name: group })).map(({ primary, related }) => {
    const name = fileName(primary.relativePath);
    return {
      name,
      companions: related.map((item) => {
        const companion = fileName(item.relativePath);
        return companion.startsWith(name) && companion !== name ? companion.slice(name.length) : companion;
      }),
    };
  });
}

// フォルダがグループとして読まれていないことの注意。Connections はグループの一覧に並べて出す。
const folderNoticeCodes = new Set([
  "group_not_declared",
  "group_directory_missing",
  "group_directory_leftover",
  "group_file_unreached",
]);

// folderNoticesOf は、その注意を選ぶ。グループでないフォルダの中のファイルは、フォルダとファイルの
// 2回知らされるので、フォルダの注意があればファイルの注意は出さない。
export function folderNoticesOf(notices: readonly Notice[]): Notice[] {
  const undeclared = notices
    .filter((notice) => notice.code === "group_not_declared" && notice.path !== undefined)
    .map((notice) => `${notice.path}/`);
  return notices.filter((notice) =>
    folderNoticeCodes.has(notice.code) &&
    !(notice.code === "group_file_unreached" && undeclared.some((folder) => notice.path?.startsWith(folder))));
}

export type MoveDirection = "up" | "down";

// ordersAfterMove は、兄弟のグループの1つを1つ動かしたあとの表示順を返す。並べ替えたことのない
// グループはどれも 0 で、読み込んだ順に並んでいるので、兄弟の全部に今の位置を番号として付ける。
export function ordersAfterMove(siblings: readonly string[], name: string, direction: MoveDirection): Map<string, number> | null {
  const from = siblings.indexOf(name);
  const to = direction === "up" ? from - 1 : from + 1;
  if (from === -1 || to < 0 || to >= siblings.length) return null;
  const next = [...siblings];
  [next[from], next[to]] = [next[to]!, next[from]!];
  return new Map(next.map((sibling, index) => [sibling, index]));
}
