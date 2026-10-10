import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { formatDuration } from "../ui/format";
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

// Each control commits on its own: a choice when it changes, a number when
// it is confirmed, the exclusion rules with their own button.
export function TransferSettingsForm({ settings, onCommit }: {
  settings: TransferSettings;
  onCommit: (settings: Partial<TransferSettings>) => void;
}) {
  const t = useTranslate();
  return <div className="space-y-6">
    <SettingsGroup heading={t("sftp.manager.settingsRecovery")}>
      <TransferRecoverySettings speedLimitBytesPerSecond={settings.speedLimitBytesPerSecond}
        autoReconnect={settings.autoReconnect} maxReconnectAttempts={settings.maxReconnectAttempts} onCommit={onCommit} />
    </SettingsGroup>
    <SettingsGroup heading={t("sftp.manager.settingsQueue")}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <label className="flex flex-col gap-1.5 text-sm text-ink-muted">
          {t("sftp.manager.concurrency")}
          <select aria-label={t("sftp.manager.concurrency")} value={settings.maxConcurrent}
            onChange={(event) => onCommit({ maxConcurrent: Number(event.target.value) })} className={selectClass}>
            {concurrencyChoices.map((choice) => <option key={choice} value={choice}>{choice}</option>)}
          </select>
        </label>
        <label className="flex flex-col gap-1.5 text-sm text-ink-muted">
          {t("sftp.manager.autoClear")}
          <select aria-label={t("sftp.manager.autoClear")} value={autoClearChoices.includes(settings.clearCompletedAfterSeconds) ? settings.clearCompletedAfterSeconds : 0}
            onChange={(event) => onCommit({ clearCompletedAfterSeconds: Number(event.target.value) })} className={selectClass}>
            {autoClearChoices.map((choice) => <option key={choice} value={choice}>{choice === 0 ? t("sftp.manager.autoClearOff") : formatDuration(choice, t)}</option>)}
          </select>
        </label>
      </div>
    </SettingsGroup>
    <SettingsGroup heading={t("sftp.manager.settingsSplit")}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <TransferIntegerSetting label={t("sftp.manager.largeFileThreshold")} value={settings.largeFileThresholdBytes}
          min={16} max={1024} scale={mebibyte} unit="MiB" onCommit={(value) => onCommit({ largeFileThresholdBytes: value })} />
        <TransferIntegerSetting label={t("sftp.manager.largeFileParallelism")} value={settings.largeFileParallelism}
          min={1} max={maxLargeFileParallelism} onCommit={(value) => onCommit({ largeFileParallelism: value })} />
        <TransferIntegerSetting label={t("sftp.manager.largeFileChunk")} value={settings.largeFileChunkBytes}
          min={8} max={4096} scale={mebibyte} unit="MiB" onCommit={(value) => onCommit({ largeFileChunkBytes: value })} />
      </div>
    </SettingsGroup>
    <div className="border-t border-line pt-3">
      <TransferExclusionSettings patterns={settings.excludePatterns ?? []} onCommit={onCommit} defaultOpen />
    </div>
  </div>;
}
