import { useRef } from "react";
import { createPortal } from "react-dom";
import { useLanguage } from "../i18n/context";
import { formatTimeOfDay } from "../ui/format";
import { Button } from "../ui/surface";
import { useCurrentTime } from "../ui/useCurrentTime";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import type { CommandBlock } from "./commandBlocks";
import { commandClockMs, commandName, commandResult } from "./commandResult";

// The card's width, and the room kept between it and the edges of the window.
const cardWidthPx = 330;
const edgePx = 8;

// TerminalCommandCard shows a command's details beside its mark in the gutter
// (commandGutter.ts): how it ended and how long it took, when and where it
// started, and ways to take its output. It opens on pointing at the mark or
// tapping it, and leaves the keyboard with the terminal.
export function TerminalCommandCard({ block, mark, onCopy, onSave, onClose, onPointerEnter, onPointerLeave }: {
  block: CommandBlock;
  // mark is where the command's mark is on the page.
  mark: DOMRect;
  onCopy: () => void;
  onSave: () => void;
  onClose: () => void;
  onPointerEnter: () => void;
  onPointerLeave: () => void;
}) {
  const { t, locale } = useLanguage();
  const now = useCurrentTime(commandClockMs);
  const card = useRef<HTMLDivElement>(null);
  useDismissibleLayer({ open: true, containerRefs: [card], onDismiss: onClose });
  const name = commandName(block, t);
  const where = [
    block.startedAt === null ? "" : t("terminal.commandStartedAt", { time: formatTimeOfDay(new Date(block.startedAt), locale) }),
    block.directory,
  ].filter((part) => part !== "").join(" · ");
  return createPortal(
    <div
      ref={card}
      role="dialog"
      aria-label={t("terminal.commandDetails", { command: name })}
      data-terminal-command-card=""
      data-status={block.status}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
      style={{
        left: Math.min(mark.right + edgePx, window.innerWidth - cardWidthPx - edgePx),
        top: Math.min(Math.max(mark.top, edgePx), window.innerHeight - 160),
        width: cardWidthPx,
      }}
      className="fixed z-[80] rounded-[10px] border border-line bg-surface px-3.5 py-3 text-[12.5px] leading-relaxed text-ink shadow-overlay"
    >
      <p className="truncate font-mono text-[13px] font-semibold" title={name}>{name}</p>
      <p className="sshc-command-result">{commandResult(block, t, now)}</p>
      {where === "" ? null : <p className="truncate text-ink-faint" title={where}>{where}</p>}
      <div className="mt-2.5 flex flex-wrap gap-2">
        <Button small onClick={onCopy}>{t("terminal.commandCopyOutput")}</Button>
        <Button small onClick={onSave}>{t("terminal.commandSaveOutput")}</Button>
      </div>
    </div>,
    document.body,
  );
}
