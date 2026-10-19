import { useRef, type PointerEvent } from "react";

// 長押しとみなすまでの時間。スマホの標準の長押し（約0.5秒）に合わせる。
const longPressMilliseconds = 450;

// useLongPress は、タッチで指を置いたまま longPressMilliseconds たったときに onLongPress を呼ぶ。
// 長押しのあとに続くクリックは consumeClick が true を返すので、呼び手はそのクリックを捨てる。
export function useLongPress() {
  const pending = useRef<{ timer: number; fired: boolean } | null>(null);

  function start(event: PointerEvent, onLongPress: () => void) {
    if (event.pointerType !== "touch") return;
    const timer = window.setTimeout(() => {
      pending.current = { timer, fired: true };
      onLongPress();
    }, longPressMilliseconds);
    pending.current = { timer, fired: false };
  }

  function cancel() {
    if (pending.current === null || pending.current.fired) return;
    window.clearTimeout(pending.current.timer);
    pending.current = null;
  }

  function consumeClick(): boolean {
    if (pending.current?.fired !== true) return false;
    pending.current = null;
    return true;
  }

  return { start, cancel, consumeClick };
}
