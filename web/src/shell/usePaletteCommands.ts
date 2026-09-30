import { vaultApi, type PasswordVaultStatus } from "../api/vault";
import type { CreateConnectionDraft } from "../connections/CreateConnectionModal";
import { useTranslate } from "../i18n/context";
import type { Section } from "../routing/sectionRoute";
import type { PaletteCommand } from "./CommandPalette";

const blankConnectionDraft: CreateConnectionDraft = {
  alias: "",
  group: "",
  hostName: "",
  user: "",
  port: "",
  authentication: "dedicated_password",
  savedCredential: "",
  newCredential: "",
  keyId: "",
};

type PaletteCommandsOptions = {
  // A Vault without a master password cannot be locked, so it has no lock command.
  passwordless: boolean;
  navigate: (section: Section) => void;
  startConnectionDraft: (draft: CreateConnectionDraft) => void;
  openLocalShell: () => void;
  // The engine answers a lock with the Vault's state: still unlocked means
  // it refused, and the screen keeps working with that state.
  onLocked: () => void;
  onStillUnlocked: (status: PasswordVaultStatus) => void;
};

// usePaletteCommands は、Ctrl/Cmd+K で移動だけでなく実行もできる操作を並べる。
// どこからでも何度かクリックしないと届かない操作を選んでいる。
export function usePaletteCommands({
  passwordless,
  navigate,
  startConnectionDraft,
  openLocalShell,
  onLocked,
  onStillUnlocked,
}: PaletteCommandsOptions): PaletteCommand[] {
  const t = useTranslate();
  return [
    {
      id: "new-connection",
      label: t("palette.newConnection"),
      detail: t("palette.newConnectionDetail"),
      search: "new connection create host add 新規 接続 追加 作成",
      run: () => {
        startConnectionDraft(blankConnectionDraft);
        navigate("Connections");
      },
    },
    {
      id: "open-files",
      label: t("palette.openRemoteFiles"),
      detail: t("palette.openRemoteFilesDetail"),
      search: "sftp files remote browse ファイル リモート 転送",
      run: () => navigate("Files"),
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
}
