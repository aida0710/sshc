import { useRef, type RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";

export function TerminalOverflowMenu({
  osc52Enabled,
  onQuickCommands,
  onPortForwarding,
  onOpenRemoteDirectory,
  onCopyContext,
  onToggleOsc52,
  onClose,
  triggerRef,
}: {
  osc52Enabled: boolean;
  onQuickCommands: () => void;
  onPortForwarding?: (() => void) | undefined;
  onOpenRemoteDirectory?: (() => void) | undefined;
  onCopyContext: () => void;
  onToggleOsc52: () => void | Promise<void>;
  onClose: () => void;
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
      <button type="button" role="menuitem" className="block min-h-10 w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0" onClick={action(onQuickCommands)}>
        {t("terminal.quickCommands")}
      </button>
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
        <span aria-hidden="true" className={osc52Enabled ? "text-live" : "text-ink-faint"}>{osc52Enabled ? "✓" : "—"}</span>
      </button>
    </div>
  );
}
