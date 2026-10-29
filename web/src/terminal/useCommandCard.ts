import { useCallback, useEffect, useRef, useState } from "react";
import type { CommandBlock } from "./commandBlocks";

// The time the card stays after the pointer leaves the mark, enough to move the pointer over to the card.
const commandCardCloseDelayMs = 250;

export type PointedCommand = { block: CommandBlock; mark: DOMRect };

// useCommandCard keeps the command card (TerminalCommandCard) open while the
// pointer is on the command's mark or on the card, and a moment after it leaves.
export function useCommandCard() {
  const [pointed, setPointed] = useState<PointedCommand | null>(null);
  const closing = useRef<number | null>(null);
  const keep = useCallback(() => {
    if (closing.current === null) return;
    window.clearTimeout(closing.current);
    closing.current = null;
  }, []);
  const point = useCallback((block: CommandBlock, mark: DOMRect) => {
    keep();
    setPointed({ block, mark });
  }, [keep]);
  const leave = useCallback(() => {
    keep();
    closing.current = window.setTimeout(() => {
      closing.current = null;
      setPointed(null);
    }, commandCardCloseDelayMs);
  }, [keep]);
  const close = useCallback(() => {
    keep();
    setPointed(null);
  }, [keep]);
  useEffect(() => keep, [keep]);
  return { pointed, point, leave, keep, close };
}
