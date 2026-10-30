import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { PanelState } from "../ui/PanelState";
import { Notice } from "../ui/surface";
import { defaultLargeFileChunkBytes, defaultLargeFileParallelism, defaultLargeFileThreshold, defaultTransferConcurrency } from "../sftp/transferDefaults";
import { TransferSettingsForm, type PresetTransferSettings } from "../sftp/TransferSettingsForm";
import { SettingsSection } from "./SettingsSection";
import { updatePreset, useSettings, type SettingsPreset } from "./settingsStore";

const defaultTransferSettings: PresetTransferSettings = {
  maxConcurrent: defaultTransferConcurrency,
  largeFileThresholdBytes: defaultLargeFileThreshold,
  largeFileParallelism: defaultLargeFileParallelism,
  largeFileChunkBytes: defaultLargeFileChunkBytes,
  speedLimitBytesPerSecond: 0,
  excludePatterns: [],
};

// storedTransferSettings leaves out every setting still at its default, so a preset holds only what was chosen.
function storedTransferSettings(settings: PresetTransferSettings): SettingsPreset["fileTransfers"] {
  const chosen = Object.fromEntries(Object.entries(settings).filter(([key, value]) =>
    JSON.stringify(value) !== JSON.stringify(defaultTransferSettings[key as keyof PresetTransferSettings])));
  return Object.keys(chosen).length === 0 ? undefined : chosen;
}

// The transfer settings of the preset being edited: how many transfers run at once, how large files are split,
// the speed limit and the exclusions. Each control saves its own change at once. The machines that use the
// preset apply a change as soon as it reaches them.
export function SFTPSettingsSection({ presetId, showHeading }: { presetId: string; showHeading: boolean }) {
  const t = useTranslate();
  const { document, loadFailed } = useSettings();
  const [failed, setFailed] = useState(false);
  const preset = document?.presets.find((candidate) => candidate.id === presetId);
  const settings: PresetTransferSettings = { ...defaultTransferSettings, ...preset?.fileTransfers };

  function commit(change: Partial<PresetTransferSettings>): Promise<boolean> {
    setFailed(false);
    return updatePreset(presetId, ({ fileTransfers: saved, ...current }) => {
      const stored = storedTransferSettings({ ...defaultTransferSettings, ...saved, ...change });
      return stored === undefined ? current : { ...current, fileTransfers: stored };
    }).then(() => true, () => {
      setFailed(true);
      return false;
    });
  }

  return (
    <SettingsSection id="settings-sftp" label={t("sftp.settingsHeading")} icon="files" showHeading={showHeading}>
      {failed ? <div className="mb-5"><Notice tone="danger" compact>{t("sftp.settingsSaveFailed")}</Notice></div> : null}
      {document === null
        ? (loadFailed ? <Notice tone="danger">{t("sftp.settingsLoadFailed")}</Notice> : <PanelState tone="loading" title={t("sftp.settingsLoading")} />)
        : <TransferSettingsForm settings={settings} onCommit={commit} />}
    </SettingsSection>
  );
}
