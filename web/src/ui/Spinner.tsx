// Spinner は、終わるまでの時間が分からない処理を待っている間に回る輪である。色は文字の色を
// 受け継ぐので、置く場所の text-* で変える。読み上げには出さず、待っていることは近くの文か
// aria-busy で伝える。動きを減らす設定のときは index.css の決まりで止まる。
export function Spinner({ className = "size-3.5" }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-block shrink-0 animate-spin rounded-full border-2 border-current border-r-transparent ${className}`}
    />
  );
}
