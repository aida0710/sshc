// 一覧で複数の接続先を選ぶときの、クリックごとの選び方を決める。
// Finder やエクスプローラと同じく、Shift は基点からの範囲、Ctrl（Mac は ⌘）は 1 件ずつの追加と解除にする。
// 接続先は identityKey の文字列で持つ。

export type MultiSelection = {
  keys: ReadonlySet<string>;
  // Shift で範囲を選ぶときの基点。最後に Ctrl で選んだか、開いていた接続先。
  anchor: string | null;
};

export const emptyMultiSelection: MultiSelection = { keys: new Set(), anchor: null };

export type SelectionGesture = "toggle" | "range";

type GestureInput = {
  current: MultiSelection;
  clicked: string;
  // エディタで開いている接続先。最初の Ctrl や Shift は、開いている 1 件も選んだものとして数える。
  opened: string | null;
  // いま一覧に見えている順の接続先。範囲はこの順で数える。
  visibleOrder: readonly string[];
  gesture: SelectionGesture;
};

export function applySelectionGesture({ current, clicked, opened, visibleOrder, gesture }: GestureInput): MultiSelection {
  if (gesture === "toggle") {
    const keys = new Set(current.keys);
    if (keys.size === 0 && opened !== null && opened !== clicked) keys.add(opened);
    if (keys.has(clicked)) keys.delete(clicked);
    else keys.add(clicked);
    return { keys, anchor: clicked };
  }
  const anchor = current.anchor ?? opened ?? clicked;
  const from = visibleOrder.indexOf(anchor);
  const to = visibleOrder.indexOf(clicked);
  if (from < 0 || to < 0) return { keys: new Set([clicked]), anchor: clicked };
  const [start, end] = from <= to ? [from, to] : [to, from];
  return { keys: new Set(visibleOrder.slice(start, end + 1)), anchor };
}

// keepVisible は、絞り込みや検索で見えなくなった接続先を選択から外す。
// 見えていないものまで動かしたり消したりしないため。何も外れなければ同じものを返す。
export function keepVisible(selection: MultiSelection, visible: ReadonlySet<string>): MultiSelection {
  const kept = [...selection.keys].filter((key) => visible.has(key));
  if (kept.length === selection.keys.size) return selection;
  const anchor = selection.anchor !== null && visible.has(selection.anchor) ? selection.anchor : null;
  return { keys: new Set(kept), anchor };
}
