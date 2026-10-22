import { useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { formatDuration } from "../ui/format";
import { Spinner } from "../ui/Spinner";
import { Notice } from "../ui/surface";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import type { TransferSettings } from "./api";
import { TransferExclusionSettings } from "./TransferExclusionSettings";
import { TransferIntegerSetting } from "./TransferIntegerSetting";
import { TransferRecoverySettings } from "./TransferRecoverySettings";

const concurrencyChoices = [1, 2, 3, 4, 5, 6, 7, 8];
const autoClearChoices = [0, 30, 300, 3600];
const mebibyte = 1 << 20;
const maxLargeFileParallelism = 128;
const selectClass = "h-11 rounded-md border border-control-line bg-control px-3 text-sm md:h-9 [@media(pointer:coarse)]:h-11";

function SettingsGroup({ heading, children }: { heading: string; children: ReactNode }) {
  return <fieldset className="min-w-0 border-t border-line pt-4">
    <legend className="pr-3 text-sm font-medium text-ink">{heading}</legend>
    {children}
  </fieldset>;
}

type CommitState = "idle" | "saving" | "saved";

// Each control commits on its own: a choice when it changes, a number when
// it is confirmed, the exclusion rules with their own button. The line above
// the groups says that the latest change is being saved and then that it was;
// editing again clears it. onCommit resolves to whether the change went
// through, and the caller says why when it did not.
export function TransferSettingsForm({ settings, onCommit }: {
  settings: TransferSettings;
  onCommit: (settings: Partial<TransferSettings>) => Promise<boolean>;
}) {
  const t = useTranslate();
  const [commitState, setCommitState] = useState<CommitState>("idle");
  // Changes can overlap, a choice made while a number is still saving. The
  // line follows the latest one.
  const commitGeneration = useRequestGeneration();

  async function commitAndReport(change: Partial<TransferSettings>): Promise<boolean> {
    const isLatest = commitGeneration.begin();
    setCommitState("saving");
    const committed = await onCommit(change);
    if (isLatest()) setCommitState(committed ? "saved" : "idle");
    return committed;
  }

  function commit(change: Partial<TransferSettings>) {
    void commitAndReport(change);
  }

  return <div className="space-y-6" onInput={() => setCommitState((current) => current === "saved" ? "idle" : current)}>
    {/* The line keeps its height when empty, so the controls below do not move while they are being used. */}
    <div className="min-h-8">
      {commitState === "saving" ? (
        <p role="status" className="flex items-center gap-2 text-xs text-ink-muted"><Spinner />{t("sftp.settingsSaving")}</p>
      ) : commitState === "saved" ? (
        <Notice tone="success" compact>{t("sftp.settingsSaved")}</Notice>
      ) : null}
    </div>
    <SettingsGroup heading={t("sftp.manager.settingsRecovery")}>
      <TransferRecoverySettings speedLimitBytesPerSecond={settings.speedLimitBytesPerSecond}
        autoReconnect={settings.autoReconnect} maxReconnectAttempts={settings.maxReconnectAttempts} onCommit={commit} />
    </SettingsGroup>
    <SettingsGroup heading={t("sftp.manager.settingsQueue")}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <label className="flex flex-col gap-1.5 text-sm text-ink-muted">
          {t("sftp.manager.concurrency")}
          <select aria-label={t("sftp.manager.concurrency")} value={settings.maxConcurrent}
            onChange={(event) => commit({ maxConcurrent: Number(event.target.value) })} className={selectClass}>
            {concurrencyChoices.map((choice) => <option key={choice} value={choice}>{choice}</option>)}
          </select>
        </label>
        <label className="flex flex-col gap-1.5 text-sm text-ink-muted">
          {t("sftp.manager.autoClear")}
          <select aria-label={t("sftp.manager.autoClear")} value={autoClearChoices.includes(settings.clearCompletedAfterSeconds) ? settings.clearCompletedAfterSeconds : 0}
            onChange={(event) => commit({ clearCompletedAfterSeconds: Number(event.target.value) })} className={selectClass}>
            {autoClearChoices.map((choice) => <option key={choice} value={choice}>{choice === 0 ? t("sftp.manager.autoClearOff") : formatDuration(choice, t)}</option>)}
          </select>
        </label>
      </div>
    </SettingsGroup>
    <SettingsGroup heading={t("sftp.manager.settingsSplit")}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <TransferIntegerSetting label={t("sftp.manager.largeFileThreshold")} value={settings.largeFileThresholdBytes}
          min={16} max={1024} scale={mebibyte} unit="MiB" onCommit={(value) => commit({ largeFileThresholdBytes: value })} />
        <TransferIntegerSetting label={t("sftp.manager.largeFileParallelism")} value={settings.largeFileParallelism}
          min={1} max={maxLargeFileParallelism} onCommit={(value) => commit({ largeFileParallelism: value })} />
        <TransferIntegerSetting label={t("sftp.manager.largeFileChunk")} value={settings.largeFileChunkBytes}
          min={8} max={4096} scale={mebibyte} unit="MiB" onCommit={(value) => commit({ largeFileChunkBytes: value })} />
      </div>
    </SettingsGroup>
    <div className="border-t border-line pt-3">
      <TransferExclusionSettings patterns={settings.excludePatterns ?? []} onCommit={commitAndReport} defaultOpen />
    </div>
  </div>;
}
