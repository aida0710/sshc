import { useTranslate } from "../i18n/context";
import { isScreenWakeLockSupported, saveKeepScreenAwake, useKeepScreenAwake } from "../terminal/keepScreenAwake";
import { useTheme } from "../theme/context";
import { themeLabel, themes } from "../theme/theme";
import type { PaletteCommand } from "./CommandPalette";

// usePaletteSettingCommands are the settings people change often, to change by name: the theme of the preset this
// machine uses, OSC 52 and keeping the screen on. Text size is among the shortcut actions.
export function usePaletteSettingCommands({ osc52Enabled, onOSC52Change }: {
  osc52Enabled: boolean;
  onOSC52Change: (enabled: boolean) => Promise<void>;
}): PaletteCommand[] {
  const t = useTranslate();
  const { theme: themeInUse, setTheme } = useTheme();
  const keepScreenAwake = useKeepScreenAwake();
  const themeCommands: PaletteCommand[] = themes.map((theme) => ({
    id: `theme:${theme}`,
    kind: "setting",
    label: t("palette.theme", { name: t(themeLabel(theme)) }),
    detail: theme === themeInUse ? t("palette.inUse") : "",
    search: `theme appearance colour color テーマ 外観 色 ${theme}`,
    run: () => setTheme(theme),
  }));
  return [
    ...themeCommands,
    {
      id: "osc52",
      kind: "setting",
      label: t(osc52Enabled ? "palette.osc52Off" : "palette.osc52On"),
      detail: t("terminal.osc52Hint"),
      search: "osc 52 clipboard クリップボード",
      run: () => { void onOSC52Change(!osc52Enabled).catch(() => undefined); },
    },
    ...(isScreenWakeLockSupported() ? [{
      id: "keep-screen-awake",
      kind: "setting" as const,
      label: t(keepScreenAwake ? "palette.keepScreenAwakeOff" : "palette.keepScreenAwakeOn"),
      detail: t("terminal.keepScreenAwakeHint"),
      search: "keep screen awake wake lock 画面 常に オン スリープ",
      run: () => saveKeepScreenAwake(!keepScreenAwake),
    }] : []),
  ];
}
