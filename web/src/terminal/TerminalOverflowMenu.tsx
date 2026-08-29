import { useEffect, useRef } from "react";
import { useTranslate } from "../i18n/context";

export function TerminalOverflowMenu({
  osc52Enabled,
  onQuickCommands,
  onCopyContext,
  onToggleOsc52,
  onClose,
}: {
  osc52Enabled: boolean;
  onQuickCommands: () => void;
  onCopyContext: () => void;
  onToggleOsc52: () => void | Promise<void>;
  onClose: () => void;
}) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const dismiss = (event: PointerEvent) => {
      if (!panel.current?.contains(event.target as Node)) onClose();
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", dismiss);
      document.removeEventListener("keydown", escape);
    };
  }, [onClose]);

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
      <button type="button" role="menuitem" className="block w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill" onClick={action(onQuickCommands)}>
        {t("terminal.quickCommands")}
      </button>
      <button type="button" role="menuitem" className="block w-full rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill" title={t("terminal.copyContextHint")} onClick={action(onCopyContext)}>
        {t("terminal.copyContext")}
      </button>
      <button
        type="button"
        role="menuitemcheckbox"
        aria-checked={osc52Enabled}
        className="flex w-full items-center justify-between gap-3 rounded px-2.5 py-2 text-left text-sm hover:bg-select-fill"
        title={t("terminal.osc52Hint")}
        onClick={action(onToggleOsc52)}
      >
        <span>OSC 52</span>
        <span aria-hidden="true" className={osc52Enabled ? "text-live" : "text-ink-faint"}>{osc52Enabled ? "✓" : "—"}</span>
      </button>
    </div>
  );
}
