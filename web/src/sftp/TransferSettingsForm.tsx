import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { sizedControl } from "../ui/form";
import { PageSection } from "../ui/page";
import { Spinner } from "../ui/Spinner";
import { useRequestGeneration } from "../ui/useRequestGeneration";
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

type CommitState = "idle" | "saving" | "saved";

// Each control commits on its own: a choice when it changes, a number when
// it is confirmed, the exclusion rules with their own button. The line beside
// the first heading says that the latest change is being saved and then that
// it was; editing again clears it. onCommit resolves to whether the change went
// through, and the caller says why when it did not.
//
// Finishing transfers and reconnecting are global settings (GeneralSettingsSection), not a preset's.
export function TransferSettingsForm({ settings, onCommit }: {
  settings: PresetTransferSettings;
  onCommit: (settings: Partial<PresetTransferSettings>) => Promise<boolean>;
}) {
  const t = useTranslate();
  const [commitState, setCommitState] = useState<CommitState>("idle");
  // Changes can overlap, a choice made while a number is still saving. The
  // line follows the latest one.
  const commitGeneration = useRequestGeneration();

  async function commitAndReport(change: Partial<PresetTransferSettings>): Promise<boolean> {
    const isLatest = commitGeneration.begin();
    setCommitState("saving");
    const committed = await onCommit(change);
    if (isLatest()) setCommitState(committed ? "saved" : "idle");
    return committed;
  }

  function commit(change: Partial<PresetTransferSettings>) {
    void commitAndReport(change);
  }

  // A line of text no taller than the heading, so the heading row does not grow when it appears.
  const commitStatus = commitState === "saving"
    ? <p role="status" className="flex items-center gap-2 text-xs text-ink-muted"><Spinner />{t("sftp.settingsSaving")}</p>
    : commitState === "saved" ? <p role="status" className="text-xs text-live">{t("sftp.settingsSaved")}</p> : undefined;

  // Editing again clears "saved". It listens to change, not input: a choice in a list fires input first, and
  // redrawing then would put the list back to the saved value before its own change handler reads the choice.
  return <div className="flex flex-col gap-3.5 md:gap-5" onChange={() => setCommitState((current) => current === "saved" ? "idle" : current)}>
    <PageSection heading={t("sftp.settingsTransfersHeading")} actions={commitStatus}>
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
      <div><TransferExclusionSettings patterns={settings.excludePatterns} onCommit={commitAndReport} /></div>
    </PageSection>
  </div>;
}
