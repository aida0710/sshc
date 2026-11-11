import { useLayoutEffect, useRef } from "react";

// 回る印（animate-spin）は、付いた瞬間から回り始める。そのままだと、同じ画面にある2つの印の向きが
// そろわない（片方は上、片方は下を向いて回る）。useSharedTurn は、印のアニメーションの始まりをページの
// 時計の 0 に置き、いつ付いた印も同じ向きで回るようにする。返す ref を回る要素に付ける。
export function useSharedTurn<Mark extends Element>() {
  const mark = useRef<Mark>(null);
  useLayoutEffect(() => {
    // 動きを減らす設定ではアニメーションが無く、何もしない。jsdom は getAnimations を持たない。
    for (const animation of mark.current?.getAnimations?.() ?? []) animation.startTime = 0;
  }, []);
  return mark;
}
