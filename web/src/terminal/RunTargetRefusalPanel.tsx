import { useId, useState } from "react";
import type { ApiError } from "../api/client";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useLanguage } from "../i18n/context";
import { formatTimeOfDay } from "../ui/format";
import { Icon, type IconName } from "../ui/icons";
import { Button } from "../ui/surface";
import { RunTargetEngineUpdate } from "./RunTargetEngineUpdate";
import { RunTargetRefusalDetails } from "./RunTargetRefusalDetails";
import { RunTargetVaultUnlock } from "./RunTargetVaultUnlock";
import { runTargetRefusalText } from "./runTargetProblem";
import { sessionListPollIntervalMs } from "./sessions";

// RunTargetRefusalPanel covers the terminal area while the run target's engine
// refuses to list its sessions. It is not a dialog: the sidebar, the run-target
// switcher and the other sections stay usable, and what fixes the refusal (the
// Vault password, the update) is done in the panel itself. The list is still
// read every few seconds, so the panel goes away by itself once the engine
// answers.
export function RunTargetRefusalPanel({
  alias,
  refusal,
  refusedAt,
  api,
  localVersion,
  vaultRevision,
  onReconnect,
  onUseThisMachine,
  onUnlocked,
  onOpenInNewTab,
  onOpenEngineInfo,
}: {
  alias: string;
  refusal: ApiError;
  // refusedAt is when the list was last refused.
  refusedAt: Date | null;
  api: RemoteEnginesApi;
  localVersion: string;
  vaultRevision: number;
  // onReconnect reads the session list again.
  onReconnect: () => Promise<void>;
  onUseThisMachine: () => void;
  onUnlocked: () => void;
  onOpenInNewTab: () => void;
  onOpenEngineInfo: () => void;
}) {
  const { t, locale } = useLanguage();
  const headingId = useId();
  const detailsId = useId();
  const [reconnecting, setReconnecting] = useState(false);
  const [detailsShown, setDetailsShown] = useState(false);
  const { heading, reason } = runTargetRefusalText(t, refusal, alias);

  async function reconnect() {
    setReconnecting(true);
    try {
      await onReconnect();
    } finally {
      setReconnecting(false);
    }
  }

  const useThisMachine = <Button onClick={onUseThisMachine}>{t("runTarget.useThisMachine")}</Button>;
  let actions;
  switch (refusal.code) {
    case "remote_engine_vault_locked":
      actions = <RunTargetVaultUnlock alias={alias} api={api} onUnlocked={onUnlocked} onOpenInNewTab={onOpenInNewTab}>{useThisMachine}</RunTargetVaultUnlock>;
      break;
    case "remote_engine_incompatible":
      actions = (
        <RunTargetEngineUpdate alias={alias} api={api} remoteVersion={refusal.problem?.detail ?? ""} localVersion={localVersion} vaultRevision={vaultRevision} onUpdated={() => void onReconnect()}>
          {useThisMachine}
        </RunTargetEngineUpdate>
      );
      break;
    default:
      actions = (
        <div className="flex flex-wrap gap-2">
          <Button kind="primary" busy={reconnecting} onClick={() => void reconnect()}>{t("runTarget.reconnect")}</Button>
          {useThisMachine}
        </div>
      );
  }

  return (
    <div className="absolute inset-0 z-30 flex items-center justify-center overflow-y-auto bg-canvas p-4">
      <section aria-labelledby={headingId} className="flex w-full max-w-lg flex-col gap-4 rounded-lg border border-line bg-card p-5 shadow-sm">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-control text-ink-muted">
            <Icon name={refusalIcon(refusal.code)} className="size-[18px]" />
          </span>
          <div className="flex min-w-0 flex-col gap-1">
            <h2 id={headingId} className="break-words text-sm font-medium text-ink">{heading}</h2>
            {reason === "" ? null : <p className="break-words text-sm leading-6 text-ink-muted">{reason}</p>}
          </div>
        </div>
        {refusedAt === null ? null : (
          <p className="flex flex-wrap gap-x-2 text-xs text-ink-muted">
            <span>{t("runTarget.lastTried")} <time dateTime={refusedAt.toISOString()} className="tabular-nums text-ink">{formatTimeOfDay(refusedAt, locale)}</time></span>
            <span aria-hidden="true">·</span>
            <span>{t("runTarget.retrying", { seconds: String(sessionListPollIntervalMs / 1_000) })}</span>
          </p>
        )}
        {actions}
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
          <button type="button" aria-expanded={detailsShown} aria-controls={detailsId} onClick={() => setDetailsShown(!detailsShown)} className="text-ink-muted underline underline-offset-2 hover:text-ink">
            {t("runTarget.details")}
          </button>
          <button type="button" onClick={onOpenEngineInfo} className="text-ink-muted underline underline-offset-2 hover:text-ink">
            {t("engineInfo.open")}
          </button>
        </div>
        {detailsShown ? <RunTargetRefusalDetails id={detailsId} refusal={refusal} /> : null}
      </section>
    </div>
  );
}

// refusalIcon pictures what stands in the way: a lock, a version to update,
// or a connection that did not reach the engine.
function refusalIcon(code: string): IconName {
  if (code === "remote_engine_vault_locked") return "secrets";
  if (code === "remote_engine_incompatible") return "arrowUp";
  return "disconnect";
}
