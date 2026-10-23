import { useId, useState } from "react";
import type { ApiError } from "../api/client";
import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { runTargetRefusalText } from "./runTargetProblem";

// RunTargetRefusalPanel covers the terminal area while the run target's engine
// refuses to list its sessions. It is not a page-wide modal: the sidebar and
// the other sections stay usable. The list is still read every few seconds, so
// the panel goes away by itself once the engine answers.
export function RunTargetRefusalPanel({
  alias,
  refusal,
  onReconnect,
  onUseThisMachine,
  onUnlock,
  onOpenEngineInfo,
}: {
  alias: string;
  refusal: ApiError;
  // onReconnect reads the session list again.
  onReconnect: () => Promise<void>;
  onUseThisMachine: () => void;
  onUnlock: () => void;
  onOpenEngineInfo: () => void;
}) {
  const t = useTranslate();
  const headingId = useId();
  const [reconnecting, setReconnecting] = useState(false);
  const { heading, reason } = runTargetRefusalText(t, refusal, alias);
  const locked = refusal.code === "remote_engine_vault_locked";
  const incompatible = refusal.code === "remote_engine_incompatible";
  // The action that fixes the refusal leads; reconnecting leads only when
  // waiting for the host is all there is to do.
  const remedy = locked
    ? <Button kind="primary" onClick={onUnlock}>{t("runTarget.unlockOpen")}</Button>
    : incompatible
      ? <Button kind="primary" onClick={onOpenEngineInfo}>{t("engineInfo.updateOpen")}</Button>
      : null;

  async function reconnect() {
    setReconnecting(true);
    try {
      await onReconnect();
    } finally {
      setReconnecting(false);
    }
  }

  return (
    <div className="absolute inset-0 z-30 flex items-center justify-center overflow-y-auto bg-canvas/80 p-4 backdrop-blur-[2px]">
      <section
        role="dialog"
        aria-labelledby={headingId}
        className="flex w-full max-w-md flex-col gap-3 rounded-lg border border-line bg-card p-4 shadow-lg"
      >
        <h2 id={headingId} className="break-words text-sm font-medium text-ink">{heading}</h2>
        {reason === "" ? null : <p className="break-words text-sm leading-6 text-ink-muted">{reason}</p>}
        <div className="flex flex-wrap gap-2">
          {remedy}
          <Button kind={remedy === null ? "primary" : "secondary"} busy={reconnecting} onClick={() => void reconnect()}>
            {t("runTarget.reconnect")}
          </Button>
          <Button onClick={onUseThisMachine}>{t("runTarget.useThisMachine")}</Button>
          {remedy === null ? <Button onClick={onOpenEngineInfo}>{t("engineInfo.open")}</Button> : null}
        </div>
      </section>
    </div>
  );
}
