import { DisclosureSummary } from "../ui/DisclosureSummary";
import { useTranslate } from "../i18n/context";
import type { TransferSettings } from "./api";
import { TransferIntegerSetting } from "./TransferIntegerSetting";

// The API stores bytes/s; the input uses an explicit binary unit.
const kibibyte = 1024;
// Keep these bounds in step with the engine's persisted settings contract.
const maximumSpeedKiB = 1 << 20;
const maximumReconnectAttempts = 10;
// Checking the box chooses a useful bounded budget when no attempt count exists.
const defaultReconnectAttempts = 3;

export function TransferRecoverySettings({ speedLimitBytesPerSecond, autoReconnect, maxReconnectAttempts, onCommit }: {
  speedLimitBytesPerSecond: number;
  autoReconnect: boolean;
  maxReconnectAttempts: number;
  onCommit: (settings: Partial<TransferSettings>) => void;
}) {
  const t = useTranslate();
  return <div className="flex flex-wrap items-center gap-3">
    <TransferIntegerSetting label={t("sftp.manager.speedLimit")} value={speedLimitBytesPerSecond}
      min={0} max={maximumSpeedKiB} scale={kibibyte} unit="KiB/s"
      onCommit={(value) => onCommit({ speedLimitBytesPerSecond: value })} />
    <span className="text-ink-muted">{t("sftp.manager.speedLimitHint")}</span>
    <label className="flex items-center gap-1 text-ink-muted">
      <input type="checkbox" checked={autoReconnect}
        onChange={(event) => onCommit({ autoReconnect: event.target.checked, maxReconnectAttempts: maxReconnectAttempts || defaultReconnectAttempts })} />
      {t("sftp.manager.autoReconnect")}
    </label>
    <TransferIntegerSetting label={t("sftp.manager.maxReconnectAttempts")} value={maxReconnectAttempts}
      min={0} max={maximumReconnectAttempts}
      onCommit={(value) => onCommit({ maxReconnectAttempts: value })} />
    <details className="basis-full text-ink-muted">
      <DisclosureSummary>{t("sftp.manager.recoveryHelp")}</DisclosureSummary>
      <p>{t("sftp.manager.recoveryHint")}</p>
    </details>
  </div>;
}
