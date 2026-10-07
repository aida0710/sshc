import { useEffect, useState } from "react";
import { defaultStack, knownFont } from "./fonts";

export function useTerminalFont(name: string, fontSize: number): string {
  const font = knownFont(name);
  const description = font === null ? "" : `${fontSize}px ${font.stack}`;
  const [loadedDescription, setLoadedDescription] = useState("");
  const fontSet = document.fonts;

  useEffect(() => {
    if (description === "" || fontSet === undefined) return;
    let active = true;
    void fontSet.load(description).then(() => {
      if (active) setLoadedDescription(description);
    }).catch(() => undefined);
    return () => { active = false; };
  }, [description, fontSet]);

  if (font === null) return defaultStack;
  // xterm caches cell measurements. Applying an unloaded font leaves those
  // measurements on the fallback even after the browser draws the new glyphs.
  const ready = fontSet === undefined || loadedDescription === description || fontSet.check(description);
  return ready ? font.stack : defaultStack;
}
