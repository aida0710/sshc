import { useCallback, useEffect, useRef, useState } from "react";

// fontSizeTipVisibleMs は、文字の大きさのティップが、最後に大きさが変わってから出ている時間である。つまむ操作や
// ホイールは続けて届くので、その間は出たままにし、止めてから「リセット」を押すのに足りる長さにしている。
export const fontSizeTipVisibleMs = 3000;

export type FontSizeTip = {
  shown: boolean;
  // show は、ティップを出し、消えるまでの時間を数え直す。
  show: () => void;
  // hold は、ポインタがティップの上にあるあいだ、消さずにおく。release で時間を数え直す。
  hold: () => void;
  release: () => void;
};

// useFontSizeTip は、ターミナルの文字の大きさを変えた直後だけ出すティップの、出す・消すを持つ。
export function useFontSizeTip(): FontSizeTip {
  const [shown, setShown] = useState(false);
  const timer = useRef<number | null>(null);
  // held は、ポインタがティップの上にあるかである。上にあるあいだは、大きさがまた変わっても消す時間を数えない。
  const held = useRef(false);
  const stopCounting = useCallback(() => {
    if (timer.current !== null) window.clearTimeout(timer.current);
    timer.current = null;
  }, []);
  const count = useCallback(() => {
    stopCounting();
    if (!held.current) timer.current = window.setTimeout(() => setShown(false), fontSizeTipVisibleMs);
  }, [stopCounting]);
  const hold = useCallback(() => {
    held.current = true;
    stopCounting();
  }, [stopCounting]);
  const release = useCallback(() => {
    held.current = false;
    count();
  }, [count]);
  const show = useCallback(() => {
    setShown(true);
    count();
  }, [count]);
  useEffect(() => stopCounting, [stopCounting]);
  return { shown, show, hold, release };
}
