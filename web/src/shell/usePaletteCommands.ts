import { vaultApi, type PasswordVaultStatus } from "../api/vault";
import { useTranslate } from "../i18n/context";
import type { Bindings } from "../keyconfig/bindings";
import type { Section } from "../routing/sectionRoute";
import { appActions, type AppActionContext } from "./appActions";
import type { PaletteCommand } from "./CommandPalette";
import { paletteActionCommands } from "./paletteActionCommands";
import { usePaletteSettingCommands } from "./usePaletteSettingCommands";
import { usePendingTerminalAction } from "./usePendingTerminalAction";

type PaletteCommandsOptions = {
  // A Vault without a master password cannot be locked, so it has no lock command.
  passwordless: boolean;
  bindings: Bindings;
  actionContext: AppActionContext;
  // removeActiveFromSplit takes the session shown out of its split, or is null outside one.
  removeActiveFromSplit: (() => void) | null;
  osc52Enabled: boolean;
  onOSC52Change: (enabled: boolean) => Promise<void>;
  navigate: (section: Section) => boolean;
  addConnection: () => void;
  openLocalShell: () => void;
  // The engine answers a lock with the Vault's state: still unlocked means
  // it refused, and the screen keeps working with that state.
  onLocked: () => void;
  onStillUnlocked: (status: PasswordVaultStatus) => void;
};

// usePaletteCommands は、Ctrl/Cmd+K で移動だけでなく実行もできる操作を並べる。commands は何も打たなくても先頭に
// 並べる操作で、どこからでも何度かクリックしないと届かないものを選んでいる。laterCommands は、ショートカットの操作と
// 設定の切り替えで、接続先などのあとに続く。
export function usePaletteCommands({
  passwordless,
  bindings,
  actionContext,
  removeActiveFromSplit,
  osc52Enabled,
  onOSC52Change,
  navigate,
  addConnection,
  openLocalShell,
  onLocked,
  onStillUnlocked,
}: PaletteCommandsOptions): { commands: PaletteCommand[]; laterCommands: PaletteCommand[] } {
  const t = useTranslate();
  const { terminalFace, activeSessionId } = actionContext;
  const runOnTerminal = usePendingTerminalAction({ terminalFace, activeSessionId, navigate });
  const settingCommands = usePaletteSettingCommands({ osc52Enabled, onOSC52Change });
  const commands: PaletteCommand[] = [
    {
      id: "new-connection",
      label: t("palette.newConnection"),
      detail: t("palette.newConnectionDetail"),
      search: "new connection create host add 新規 接続 追加 作成",
      run: addConnection,
    },
    {
      id: "open-files",
      label: t("palette.openRemoteFiles"),
      detail: t("palette.openRemoteFilesDetail"),
      search: "sftp files remote browse ファイル リモート 転送",
      keys: bindings.sftp,
      run: () => { navigate("Files"); },
    },
    {
      id: "open-shell",
      label: t("palette.openLocalShell"),
      detail: t("palette.openLocalShellDetail"),
      search: "shell local terminal console シェル ローカル ターミナル",
      run: openLocalShell,
    },
    ...(passwordless ? [] : [{
      id: "lock-vault",
      label: t("palette.lockVault"),
      detail: t("palette.lockVaultDetail"),
      search: "lock vault secure ロック 保管庫 施錠",
      run: () => {
        void vaultApi.lockVault().then((status) => {
          if (status.unlocked) onStillUnlocked(status);
          else onLocked();
        }).catch(() => undefined);
      },
    }]),
  ];
  const laterCommands: PaletteCommand[] = [
    // From the palette an action of the terminal screen shows that screen first.
    ...paletteActionCommands({ t, bindings, appActions: appActions({ ...actionContext, terminalFace: true }), hasSession: activeSessionId !== null, runOnTerminal }),
    ...(removeActiveFromSplit === null ? [] : [{
      id: "remove-from-split",
      label: t("splitView.remove"),
      detail: "",
      search: "remove from split pane 分割 外す ペイン",
      run: removeActiveFromSplit,
    }]),
    ...settingCommands,
  ];
  return { commands, laterCommands };
}
