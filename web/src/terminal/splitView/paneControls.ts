import type { DragEvent } from "react";

// MovingPane は、クリックで入れ替えるペインを選んでいる途中に、どのペインを選んだかである。this はこのペイン、
// other はほかのペイン（このペインで［ここと入れ替える］を選ぶと入れ替わる）、null は選んでいない。
export type MovingPane = "this" | "other" | null;

// PaneDragHandlers は、見出しをつかんでペインを動かす操作の始まりと終わりである（usePaneDrag の begin と clear）。
export type PaneDragHandlers = {
  onDragStart: (event: DragEvent<HTMLElement>) => void;
  onDragEnd: () => void;
};

// SplitPaneControls は、分割表示に並んだペインの操作である。モバイル幅でペインを1つずつ出すあいだは無い。
// 見出しには［×］（と集中表示の間の［集中表示をやめる］）を出し、入れ替えと集中表示は［…］に入れる。
export type SplitPaneControls = {
  moving: MovingPane;
  // focusMode は、分割表示のペインのうち1つだけを集中表示しているか。
  focusMode: boolean;
  onMove: () => void;
  onFocusMode: () => void;
  onRemove: () => void;
  // drag は、見出しをつかんでペインを動かす操作である。集中表示の間は、落とす先のペインが見えないので無い。
  // タッチで使う設定でも無い。見出しの名前は、長押しやなぞりで文字を選べるようにしておく（動かすのは［…］から）。
  drag: PaneDragHandlers | undefined;
};
