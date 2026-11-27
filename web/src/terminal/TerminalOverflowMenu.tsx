import type { RefObject } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { MenuChoiceItem, MenuGroup, MenuItem } from "../ui/MenuItems";
import { MenuPanel } from "../ui/MenuPanel";
import { isScreenWakeLockSupported, saveKeepScreenAwake, useKeepScreenAwake } from "./keepScreenAwake";
import { saveTouchMode, useDetectedTouchMode, useTouchMode, type FixedTouchMode } from "./touchMode";
import type { PaneEdge } from "./splitView/layout";
import { useSettingScopeLabel } from "../settings/SettingScopeTag";

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
  splitRefusal,
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
  // splitRefusal は、このターミナルの edge の側に何も置けない理由である。置けるときは undefined。
  splitRefusal?: ((edge: PaneEdge) => string | undefined) | undefined;
  triggerRef: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const scopeLabel = useSettingScopeLabel();
  const keepScreenAwake = useKeepScreenAwake();
  const touchMode = useTouchMode();
  const detectedTouchMode = useDetectedTouchMode();

  const action = (callback: () => void | Promise<void>) => () => {
    void callback();
    onClose();
  };

  return (
    <MenuPanel label={t("terminal.moreActions")} anchorRef={triggerRef} width="wide" onClose={onClose}>
      {onSearch === undefined ? null : (
        <MenuItem onSelect={action(onSearch)}>
          {t("terminal.search")}
        </MenuItem>
      )}
      {onPreviousPrompt === undefined ? null : (
        <MenuItem onSelect={action(onPreviousPrompt)}>
          {t("shortcuts.previousPrompt")}
        </MenuItem>
      )}
      {onNextPrompt === undefined ? null : (
        <MenuItem onSelect={action(onNextPrompt)}>
          {t("shortcuts.nextPrompt")}
        </MenuItem>
      )}
      {onSplit === undefined ? null : (["right", "bottom"] as const).map((edge) => {
        const refusal = splitRefusal?.(edge);
        return (
          <MenuItem key={edge} disabled={refusal !== undefined} title={refusal}
            onSelect={action(() => onSplit(edge))}>{t(edge === "right" ? "splitView.right" : "splitView.below")}</MenuItem>
        );
      })}
      {onRemoveFromSplit === undefined ? null : (
        <MenuItem onSelect={action(onRemoveFromSplit)}>
          {t("splitView.remove")}
        </MenuItem>
      )}
      {onQuickCommands === undefined ? null : (
        <MenuItem onSelect={action(onQuickCommands)}>
          {t("terminal.quickCommands")}
        </MenuItem>
      )}
      {onPortForwarding === undefined ? null : (
        <MenuItem onSelect={action(onPortForwarding)}>
          {t("terminal.portForwarding")}
        </MenuItem>
      )}
      {onOpenRemoteDirectory === undefined ? null : (
        <MenuItem onSelect={action(onOpenRemoteDirectory)}>
          {t("terminal.openDirectoryInSFTP")}
        </MenuItem>
      )}
      <MenuItem title={t("terminal.copyContextHint")} onSelect={action(onCopyContext)}>
        {t("terminal.copyContext")}
      </MenuItem>
      <MenuItem title={t("terminal.copyWithColorsHint")} onSelect={action(onCopyWithColors)}>
        {t("terminal.copyWithColors")}
      </MenuItem>
      <MenuItem onSelect={action(onSaveScrollback)}>
        {t("terminal.saveScrollback")}
      </MenuItem>
      {/* These apply to every terminal, not only this one, so they sit apart, each group under how far it
          reaches: OSC 52 is one value for every machine, the rest belong to the preset this machine uses. */}
      <MenuGroup heading={scopeLabel("allMachines")}>
        <MenuChoiceItem role="menuitemcheckbox" label="OSC 52" hint={t("terminal.osc52Hint")} checked={osc52Enabled} onSelect={action(onToggleOsc52)} />
      </MenuGroup>
      <MenuGroup heading={scopeLabel("preset")}>
        {isScreenWakeLockSupported() ? (
          <MenuChoiceItem role="menuitemcheckbox" label={t("terminal.keepScreenAwake")} hint={t("terminal.keepScreenAwakeHint")} checked={keepScreenAwake}
            onSelect={action(() => saveKeepScreenAwake(!keepScreenAwake))} />
        ) : null}
        {/* Turning detection off keeps the menu open: the two fixed choices appear under it to pick from.
            It starts on the one detection picked, so nothing changes until one is picked. */}
        <MenuChoiceItem role="menuitemcheckbox" label={t("terminal.touchModeAuto")} hint={t("terminal.touchModeAutoHint")} checked={touchMode === "auto"}
          onSelect={() => saveTouchMode(touchMode === "auto" ? detectedTouchMode : "auto")} />
        {touchMode === "auto" ? null : fixedTouchModes.map(({ mode, label, hint }) => (
          <MenuChoiceItem key={mode} role="menuitemradio" indent label={t(label)} hint={t(hint)} checked={touchMode === mode}
            onSelect={action(() => saveTouchMode(mode))} />
        ))}
      </MenuGroup>
    </MenuPanel>
  );
}

const fixedTouchModes: { mode: FixedTouchMode; label: MessageKey; hint: MessageKey }[] = [
  { mode: "touch", label: "terminal.touchModeTouch", hint: "terminal.touchModeTouchHint" },
  { mode: "mouse", label: "terminal.touchModeMouse", hint: "terminal.touchModeMouseHint" },
];
