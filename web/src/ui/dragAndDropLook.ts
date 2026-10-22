// ドラッグできる所と落とせる所の見た目を、どの画面でも同じにするための class である。
// 手本は Keys（鍵の取っ手とフォルダ）。

// ドラッグできる要素のカーソル。押しているあいだは掴んでいる形にする。
// ブラウザはボタンに既定のカーソルを付けて親から受け継がないので、ドラッグできる行の中で
// 行の大部分を覆うボタンにも付ける。
export const draggableCursor = "cursor-grab active:cursor-grabbing";

// ドラッグ中に落とせる場所は点線で示し、ポインタが真上にある1つだけを実線と背景で強める。
const dropTargetAvailable = "outline outline-1 outline-dashed outline-control-line";
const dropTargetOver = "bg-accent/20 outline-2 outline-solid outline-accent";

// dropTargetLook は、落とせる場所の今の見た目を返す。落とせないときは何も足さない。
export function dropTargetLook({ accepts, over }: { accepts: boolean; over: boolean }): string {
  if (!accepts) return "";
  return over ? dropTargetOver : dropTargetAvailable;
}
