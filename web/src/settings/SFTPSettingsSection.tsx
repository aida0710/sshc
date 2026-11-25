import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField } from "../ui/form";
import { PageSection } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Notice } from "../ui/surface";
import { defaultLargeFileChunkBytes, defaultLargeFileParallelism, defaultLargeFileThreshold, defaultTransferConcurrency } from "../sftp/transferDefaults";
import { TransferSettingsForm, type PresetTransferSettings } from "../sftp/TransferSettingsForm";
import { presetInterfaceOf } from "./presetInterface";
import { SettingsSection } from "./SettingsSection";
import { usePresetInterfaceEdit, useSettingsDraft } from "./settingsDraftContext";
import type { SettingsPreset } from "./settingsStore";

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

// The SFTP settings of the preset being edited: whether lists draw thumbnails, how many transfers run at once, how
// large files are split, the speed limit and the exclusions. The machines that use the preset apply a saved change as
// soon as it reaches them.
export function SFTPSettingsSection({ presetId, showHeading }: { presetId: string; showHeading: boolean }) {
  const t = useTranslate();
  const { document, loadFailed, editPreset } = useSettingsDraft();
  const editInterface = usePresetInterfaceEdit(presetId);
  const preset = document?.presets.find((candidate) => candidate.id === presetId);
  const settings: PresetTransferSettings = { ...defaultTransferSettings, ...preset?.fileTransfers };

  function change<Field extends keyof PresetTransferSettings>(field: Field, value: PresetTransferSettings[Field]) {
    editPreset(presetId, `fileTransfers.${field}`, ({ fileTransfers: saved, ...current }) => {
      const stored = storedTransferSettings({ ...defaultTransferSettings, ...saved, [field]: value });
      return stored === undefined ? current : { ...current, fileTransfers: stored };
    });
  }

  return (
    <SettingsSection id="settings-sftp" label={t("sftp.settingsHeading")} icon="files" showHeading={showHeading}>
      {document === null
        ? (loadFailed ? <Notice tone="danger">{t("sftp.settingsLoadFailed")}</Notice> : <PanelState tone="loading" title={t("sftp.settingsLoading")} />)
        : (
          <div className="flex flex-col gap-3.5 md:gap-5">
            <PageSection heading={t("sftp.settingsListHeading")}>
              <FieldRow label={t("sftp.settingsThumbnailsRow")} interactiveChildren>
                <CheckboxField
                  label={t("sftp.settingsThumbnails")}
                  hint={t("sftp.settingsThumbnailsHint")}
                  checked={presetInterfaceOf(document, presetId).hideSFTPThumbnails !== true}
                  onChange={(checked) => editInterface("hideSFTPThumbnails", (current) => ({ ...current, hideSFTPThumbnails: !checked }))}
                />
              </FieldRow>
            </PageSection>
            <TransferSettingsForm settings={settings} onChange={change} />
          </div>
        )}
    </SettingsSection>
  );
}
