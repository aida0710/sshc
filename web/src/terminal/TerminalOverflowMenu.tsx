import { useRef, type RefObject } from "react";
import { Icon } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { menuChoiceItemClass, menuGroupClass, menuGroupHeadingClass, menuItemClass, menuSurface } from "../ui/menuStyle";
import { isScreenWakeLockSupported, saveKeepScreenAwake, useKeepScreenAwake } from "./keepScreenAwake";
import { saveTouchMode, useDetectedTouchMode, useTouchMode, type FixedTouchMode } from "./touchMode";
import type { PaneEdge } from "./splitView/layout";

export function TerminalOverflowMenu({
  osc52Enabled,
  onQuickCommands,
  onPortForwarding,
  onOpenRemoteDirectory,
  onCopyContext,
  onCopyWithColors,
  onSaveScrollback,
  onToggleOsc52,
  onClose,
  onSplit,
  onRemoveFromSplit,
  onSearch,
  onPreviousPrompt,
  onNextPrompt,
  canSplit = false,
  triggerRef,
}: {
  osc52Enabled: boolean;
  onQuickCommands?: (() => void) | undefined;
  onPortForwarding?: (() => void) | undefined;
  onOpenRemoteDirectory?: (() => void) | undefined;
  onCopyContext: () => void;
  onCopyWithColors: () => void;
  onSaveScrollback: () => void;
  onToggleOsc52: () => void | Promise<void>;
  onClose: () => void;
  onSplit?: ((edge: PaneEdge) => void) | undefined;
  // onRemoveFromSplit is offered while the terminal is in a split. The session goes on outside it.
  onRemoveFromSplit?: (() => void) | undefined;
  // onSearch is offered here on a phone, whose top bar keeps only the other actions.
  onSearch?: (() => void) | undefined;
  // The previous and next prompt, offered on a phone, which has no keys for them.
  onPreviousPrompt?: (() => void) | undefined;
  onNextPrompt?: (() => void) | undefined;
  canSplit?: boolean;
  triggerRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);
  const keepScreenAwake = useKeepScreenAwake();
  const touchMode = useTouchMode();
  const detectedTouchMode = useDetectedTouchMode();

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
      className={`absolute right-2 top-[calc(100%-0.25rem)] z-40 w-64 ${menuSurface}`}
    >
      {onSearch === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onSearch)}>
          {t("terminal.search")}
        </button>
      )}
      {onPreviousPrompt === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onPreviousPrompt)}>
          {t("shortcuts.previousPrompt")}
        </button>
      )}
      {onNextPrompt === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onNextPrompt)}>
          {t("shortcuts.nextPrompt")}
        </button>
      )}
      {onSplit === undefined ? null : (["right", "bottom"] as const).map((edge) => (
        <button key={edge} type="button" role="menuitem" disabled={!canSplit} className={menuItemClass()}
          onClick={action(() => onSplit(edge))}>{t(edge === "right" ? "splitView.right" : "splitView.below")}</button>
      ))}
      {onRemoveFromSplit === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onRemoveFromSplit)}>
          {t("splitView.remove")}
        </button>
      )}
      {onQuickCommands === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onQuickCommands)}>
          {t("terminal.quickCommands")}
        </button>
      )}
      {onPortForwarding === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onPortForwarding)}>
          {t("terminal.portForwarding")}
        </button>
      )}
      {onOpenRemoteDirectory === undefined ? null : (
        <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onOpenRemoteDirectory)}>
          {t("terminal.openDirectoryInSFTP")}
        </button>
      )}
      <button type="button" role="menuitem" className={menuItemClass()} title={t("terminal.copyContextHint")} onClick={action(onCopyContext)}>
        {t("terminal.copyContext")}
      </button>
      <button type="button" role="menuitem" className={menuItemClass()} title={t("terminal.copyWithColorsHint")} onClick={action(onCopyWithColors)}>
        {t("terminal.copyWithColors")}
      </button>
      <button type="button" role="menuitem" className={menuItemClass()} onClick={action(onSaveScrollback)}>
        {t("terminal.saveScrollback")}
      </button>
      {/* These apply to every terminal, not only this one, so they sit apart under their own heading. */}
      <div role="group" aria-label={t("terminal.globalSettings")} className={menuGroupClass}>
        <p aria-hidden="true" className={menuGroupHeadingClass}>{t("terminal.globalSettings")}</p>
        <ChoiceMenuItem role="menuitemcheckbox" label="OSC 52" hint={t("terminal.osc52Hint")} checked={osc52Enabled} onSelect={action(onToggleOsc52)} />
        {isScreenWakeLockSupported() ? (
          <ChoiceMenuItem role="menuitemcheckbox" label={t("terminal.keepScreenAwake")} hint={t("terminal.keepScreenAwakeHint")} checked={keepScreenAwake}
            onSelect={action(() => saveKeepScreenAwake(!keepScreenAwake))} />
        ) : null}
        {/* Turning detection off keeps the menu open: the two fixed choices appear under it to pick from.
            It starts on the one detection picked, so nothing changes until one is picked. */}
        <ChoiceMenuItem role="menuitemcheckbox" label={t("terminal.touchModeAuto")} hint={t("terminal.touchModeAutoHint")} checked={touchMode === "auto"}
          onSelect={() => saveTouchMode(touchMode === "auto" ? detectedTouchMode : "auto")} />
        {touchMode === "auto" ? null : fixedTouchModes.map(({ mode, label, hint }) => (
          <ChoiceMenuItem key={mode} role="menuitemradio" indent label={t(label)} hint={t(hint)} checked={touchMode === mode}
            onSelect={action(() => saveTouchMode(mode))} />
        ))}
      </div>
    </div>
  );
}

const fixedTouchModes: { mode: FixedTouchMode; label: MessageKey; hint: MessageKey }[] = [
  { mode: "touch", label: "terminal.touchModeTouch", hint: "terminal.touchModeTouchHint" },
  { mode: "mouse", label: "terminal.touchModeMouse", hint: "terminal.touchModeMouseHint" },
];

function ChoiceMenuItem({ role, label, hint, checked, onSelect, indent = false }: {
  role: "menuitemcheckbox" | "menuitemradio";
  label: string;
  hint: string;
  checked: boolean;
  onSelect: () => void;
  indent?: boolean;
}) {
  return (
    <button
      type="button"
      role={role}
      aria-checked={checked}
      className={menuChoiceItemClass({ indent })}
      title={hint}
      onClick={onSelect}
    >
      <span>{label}</span>
      <Icon name={checked ? "check" : "minus"} className={`size-4 ${checked ? "text-live" : "text-ink-faint"}`} />
    </button>
  );
}
