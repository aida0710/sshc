// 指が動き出した向き。1本の指の動きを、横になぞる操作（スマホのナビゲーションの出し入れ）と、上下に動かす
// 操作（ターミナルのスクロール）のどちらか一方だけが受け取るように、両方がここで同じ決め方をする。別々に
// 決めると、ナビゲーションを引き出す指の上下のぶれでターミナルも動く。
export type SwipeDirection = "undecided" | "sideways" | "upDown";

// 向きを決めるまでに指が動く距離。これより短い動きはタップのままにする。
const directionThreshold = 10;
// 横の動きが上下の動きのこの倍より大きいときに、横になぞったと読む。少し斜めに上下へ動かす指で
// ナビゲーションが出ないようにする。
const sidewaysRatio = 1.2;

// swipeDirection は、指が触れた点から across（右が正）・down（下が正）だけ動いたときの向きを返す。
export function swipeDirection(across: number, down: number): SwipeDirection {
  if (Math.abs(across) < directionThreshold && Math.abs(down) < directionThreshold) return "undecided";
  return Math.abs(across) > Math.abs(down) * sidewaysRatio ? "sideways" : "upDown";
}
