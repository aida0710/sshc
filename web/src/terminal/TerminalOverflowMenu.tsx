import { useRef, type RefObject } from "react";
import { Icon } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import type { PaneEdge } from "./splitView/layout";

export function TerminalOverflowMenu({
  osc52Enabled,
  onQuickCommands,
  onPortForwarding,
  onOpenRemoteDirectory,
  onCopyContext,
  onToggleOsc52,
  onClose,
  onSplit,
  onSearch,
  canSplit = false,
  triggerRef,
}: {
  osc52Enabled: boolean;
  onQuickCommands?: (() => void) | undefined;
  onPortForwarding?: (() => void) | undefined;
  onOpenRemoteDirectory?: (() => void) | undefined;
  onCopyContext: () => void;
  onToggleOsc52: () => void | Promise<void>;
  onClose: () => void;
  onSplit?: ((edge: PaneEdge) => void) | undefined;
  // onSearch is offered here on a phone, whose top bar keeps only the other actions.
  onSearch?: (() => void) | undefined;
  canSplit?: boolean;
  triggerRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);

  useDismissibleLayer({
    open: true,
    containerRefs: triggerRef === undefined ? [panel] : [panel, triggerRef],
    onDismiss: onClose,
    ...(triggerRef === undefined ? {} : { returnFocusRef: triggerRef }),
  });
  useMenuKeyboard({ open: true, menuRef: panel, onClose });

  const action = (callback: () => void | Promise<void>) => () => {
    void callback();
    onClose();
  };

  return (
    <div
      ref={panel}
      role="menu"
      aria-label={t("terminal.moreActions")}
      className="absolute right-2 top-[calc(100%-0.25rem)] z-40 w-64 rounded-md border border-control-line bg-card p-1.5 shadow-2xl"
    >
      {onSearch === undefined ? null : (
        <button type="button" role="menuitem" className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0" onClick={action(onSearch)}>
          {t("terminal.search")}
        </button>
      )}
      {onSplit === undefined ? null : (["right", "bottom"] as const).map((edge) => (
        <button key={edge} type="button" role="menuitem" disabled={!canSplit}
          className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:opacity-50 md:min-h-0"
          onClick={action(() => onSplit(edge))}>{t(edge === "right" ? "splitView.right" : "splitView.below")}</button>
      ))}
      {onQuickCommands === undefined ? null : (
        <button type="button" role="menuitem" className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0" onClick={action(onQuickCommands)}>
          {t("terminal.quickCommands")}
        </button>
      )}
      {onPortForwarding === undefined ? null : (
        <button type="button" role="menuitem" className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0" onClick={action(onPortForwarding)}>
          {t("terminal.portForwarding")}
        </button>
      )}
      {onOpenRemoteDirectory === undefined ? null : (
        <button type="button" role="menuitem" className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0" onClick={action(onOpenRemoteDirectory)}>
          {t("terminal.openDirectoryInSFTP")}
        </button>
      )}
      <button type="button" role="menuitem" className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0" title={t("terminal.copyContextHint")} onClick={action(onCopyContext)}>
        {t("terminal.copyContext")}
      </button>
      <button
        type="button"
        role="menuitemcheckbox"
        aria-checked={osc52Enabled}
        className="flex min-h-10 w-full items-center justify-between gap-3 rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0"
        title={t("terminal.osc52Hint")}
        onClick={action(onToggleOsc52)}
      >
        <span>OSC 52</span>
        <Icon name={osc52Enabled ? "check" : "minus"} className={`size-4 ${osc52Enabled ? "text-live" : "text-ink-faint"}`} />
      </button>
    </div>
  );
}
