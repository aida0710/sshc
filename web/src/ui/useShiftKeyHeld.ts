import { useEffect, useState } from "react";

// useShiftKeyHeld は、Shift キーが押されたままかを返す。ウィンドウがフォーカスを
// 失ったときやタブが隠れたときは keyup が届かないので、離したものとして扱う。
export function useShiftKeyHeld(): boolean {
  const [held, setHeld] = useState(false);
  useEffect(() => {
    function keyDown(event: KeyboardEvent) {
      if (event.key === "Shift") setHeld(true);
    }
    function keyUp(event: KeyboardEvent) {
      if (event.key === "Shift") setHeld(false);
    }
    function release() {
      setHeld(false);
    }
    function releaseWhenHidden() {
      if (document.hidden) release();
    }
    window.addEventListener("keydown", keyDown);
    window.addEventListener("keyup", keyUp);
    window.addEventListener("blur", release);
    document.addEventListener("visibilitychange", releaseWhenHidden);
    return () => {
      window.removeEventListener("keydown", keyDown);
      window.removeEventListener("keyup", keyUp);
      window.removeEventListener("blur", release);
      document.removeEventListener("visibilitychange", releaseWhenHidden);
    };
  }, []);
  return held;
}
