import { useCallback, useSyncExternalStore } from "react";
import { useTranslate } from "../i18n/context";
import { Notice } from "../ui/surface";
import { sftpTransferManager } from "../sftp/transferManager";
import { TransferSettingsForm } from "../sftp/TransferSettingsForm";
import { useTransferControl } from "../sftp/useTransferControl";
import { SettingsLoadFailure } from "./SettingsLoadFailure";
import { SettingsSection } from "./SettingsSection";
import { useSettingsLoad } from "./useSettingsLoad";

// The transfer settings are part of the engine's transfer queue, so they are
// read from the queue listing and each control saves its own change at once.
export function SFTPSettingsSection({ showHeading }: { showHeading: boolean }) {
  const t = useTranslate();
  // The app lists the queue every few seconds; each listing may carry settings
  // changed in another tab, so this section follows the listing too.
  useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  // A fresh listing before the form opens, so that the first change is not
  // sent together with defaults that were never read from the engine.
  const loadSettings = useCallback(() => sftpTransferManager.reconcile(), []);
  const settingsLoad = useSettingsLoad(loadSettings);
  const { problem, runControl } = useTransferControl();
  return (
    <SettingsSection id="settings-sftp" label={t("sftp.settingsHeading")} icon="files" showHeading={showHeading}>
      <div className="max-w-2xl">
        <SettingsLoadFailure settingsLoad={settingsLoad} title={t("sftp.settingsLoadFailed")} />
        {problem === "" ? null : <div className="mb-5"><Notice tone="danger" compact>{problem}</Notice></div>}
        {settingsLoad.state === "loaded" ? (
          <TransferSettingsForm settings={sftpTransferManager.getSettings()}
            onCommit={(change) => runControl(() => sftpTransferManager.applySettings(change))} />
        ) : null}
      </div>
    </SettingsSection>
  );
}
