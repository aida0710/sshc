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
