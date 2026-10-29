import { useTranslate } from "../i18n/context";
import { useCurrentTime } from "../ui/useCurrentTime";
import type { CommandBlock } from "./commandBlocks";
import { commandName, commandResult, commandClockMs } from "./commandResult";

// TerminalCommandHeader keeps the command in sight above the top of the view
// while its long output is scrolled back through. Pressing it goes to the
// command's line.
export function TerminalCommandHeader({ block, onGoTo }: { block: CommandBlock; onGoTo: () => void }) {
  const t = useTranslate();
  const now = useCurrentTime(commandClockMs);
  const name = commandName(block, t);
  return (
    <button
      type="button"
      data-terminal-command-header=""
      data-status={block.status}
      aria-label={t("terminal.commandGoTo", { command: name })}
      onClick={onGoTo}
      className="absolute inset-x-0 top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-[color-mix(in_srgb,var(--ui-term-bg)_93%,var(--ui-ink))] px-2 py-1 text-left shadow-[0_4px_10px_rgb(0_0_0/0.3)]"
    >
      <span className="min-w-0 truncate font-mono text-[13px] text-[var(--ui-term-fg)]">$ {name}</span>
      <span className="sshc-command-result shrink-0 text-xs">{commandResult(block, t, now)}</span>
    </button>
  );
}
