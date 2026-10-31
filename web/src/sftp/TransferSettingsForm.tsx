import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { sizedControl } from "../ui/form";
import { PageSection } from "../ui/page";
import type { SettingsPreset } from "../settings/settingsStore";
import { TransferExclusionSettings } from "./TransferExclusionSettings";
import { IntegerSetting } from "../ui/IntegerSetting";

const concurrencyChoices = [1, 2, 3, 4, 5, 6, 7, 8];
const mebibyte = 1 << 20;
const maxLargeFileParallelism = 128;
// The API stores bytes/s; the input uses an explicit binary unit.
const kibibyte = 1024;
// Keep these bounds in step with the engine's persisted settings contract.
const maximumSpeedKiB = 1 << 20;

// PresetTransferSettings is a preset's transfer settings with the engine's defaults filled in.
export type PresetTransferSettings = Required<NonNullable<SettingsPreset["fileTransfers"]>>;

// Each control commits on its own: a choice when it changes, a number and the exclusion rules when they are
// confirmed (left, or Enter). The settings page says that a change was saved (SettingsSavedMark). onCommit
// resolves to whether the change went through, and the caller says why when it did not.
//
// Finishing transfers and reconnecting are global settings (GeneralSettingsSection), not a preset's.
export function TransferSettingsForm({ settings, onCommit }: {
  settings: PresetTransferSettings;
  onCommit: (settings: Partial<PresetTransferSettings>) => Promise<boolean>;
}) {
  const t = useTranslate();

  function commit(change: Partial<PresetTransferSettings>) {
    void onCommit(change);
  }

  return <div className="flex flex-col gap-3.5 md:gap-5">
    <PageSection heading={t("sftp.settingsTransfersHeading")}>
      {/* The rows sit in their own box so the first one, right under the heading, has no line above it. */}
      <div>
        <FieldRow label={t("sftp.manager.speedLimit")} hint={t("sftp.manager.speedLimitHint")}>
          <IntegerSetting label={t("sftp.manager.speedLimit")} value={settings.speedLimitBytesPerSecond}
            min={0} max={maximumSpeedKiB} scale={kibibyte} unit="KiB/s" widthClass="w-[110px]"
            onCommit={(value) => commit({ speedLimitBytesPerSecond: value })} />
        </FieldRow>
        <FieldRow label={t("sftp.manager.concurrency")}>
          <select aria-label={t("sftp.manager.concurrency")} value={settings.maxConcurrent} onChange={(event) => commit({ maxConcurrent: Number(event.target.value) })}
            className={sizedControl("w-[120px]")}>
            {concurrencyChoices.map((choice) => <option key={choice} value={choice}>{choice}</option>)}
          </select>
        </FieldRow>
        {/* The three numbers read as one sentence; each input keeps its own name. */}
        <FieldRow label={t("sftp.manager.split")} interactiveChildren>
          <span className="flex flex-wrap items-center gap-2">
            <IntegerSetting label={t("sftp.manager.largeFileThreshold")} value={settings.largeFileThresholdBytes}
              min={16} max={1024} scale={mebibyte} unit={t("sftp.manager.splitAfterThreshold")} widthClass="w-[80px]"
              onCommit={(value) => commit({ largeFileThresholdBytes: value })} />
            <IntegerSetting label={t("sftp.manager.largeFileParallelism")} value={settings.largeFileParallelism}
              min={1} max={maxLargeFileParallelism} unit={t("sftp.manager.splitAfterParallelism")} widthClass="w-[60px]"
              onCommit={(value) => commit({ largeFileParallelism: value })} />
            <IntegerSetting label={t("sftp.manager.largeFileChunk")} value={settings.largeFileChunkBytes}
              min={8} max={4096} scale={mebibyte} unit={t("sftp.manager.splitAfterChunk")} widthClass="w-[70px]"
              onCommit={(value) => commit({ largeFileChunkBytes: value })} />
          </span>
        </FieldRow>
      </div>
    </PageSection>
    <PageSection heading={t("sftp.manager.exclusions")} hint={t("sftp.manager.exclusionsHint")}>
      <div><TransferExclusionSettings patterns={settings.excludePatterns} onCommit={onCommit} /></div>
    </PageSection>
  </div>;
}
