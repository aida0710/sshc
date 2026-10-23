import { useId, useRef, useState, type RefObject } from "react";
import type { RemoteEngineAction, RemoteEngineStatus } from "../api/remoteEngines";
import { useTranslate, type Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { useEngineInfo } from "../terminal/useEngineInfo";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { ModalShell } from "../ui/ModalShell";
import { Button, Notice } from "../ui/surface";

// engineActions are listed in the order the dialog offers them.
const engineActions: RemoteEngineAction[] = ["update", "restart", "stop"];

// actionLabels name each action: its button, the button while it runs, the
// confirmation heading and the line once the host's sshc succeeded.
const actionLabels: Record<RemoteEngineAction, { button: MessageKey; running: MessageKey; confirm: MessageKey; done: MessageKey }> = {
  update: { button: "engineInfo.update", running: "engineInfo.updating", confirm: "engineInfo.confirmUpdate", done: "engineInfo.updated" },
  restart: { button: "engineInfo.restart", running: "engineInfo.restarting", confirm: "engineInfo.confirmRestart", done: "engineInfo.restarted" },
  stop: { button: "engineInfo.stop", running: "engineInfo.stopping", confirm: "engineInfo.confirmStop", done: "engineInfo.stopped" },
};

// EngineInfoDialog shows a run-target engine's version, Vault and sessions,
// and stops, restarts or updates it. Each action stops every terminal on that
// engine, so it is confirmed first.
export function EngineInfoDialog({
  alias,
  targets,
  onClose,
  returnFocusRef,
}: {
  alias: string;
  targets: RunTargetsState;
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const headingId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const info = useEngineInfo({
    alias, api: targets.api, translate: t, vaultRevision: targets.vaultUnlock.revision,
  });
  const [confirming, setConfirming] = useState<RemoteEngineAction | null>(null);
  const busy = info.running !== null;

  return (
    <>
      <ModalShell
        labelledBy={headingId}
        onDismiss={onClose}
        dismissible={!busy}
        initialFocusRef={closeRef}
        {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
        panelClassName="flex w-full max-w-md flex-col gap-3 rounded-lg p-4"
      >
        <h2 id={headingId} className="text-sm font-medium text-ink">
          {t("engineInfo.heading", { alias })}
        </h2>
        {info.status === null && info.remoteVersion === null ? null : (
          <EngineFacts status={info.status} remoteVersion={info.remoteVersion} localVersion={targets.localVersion} t={t} onUnlock={busy ? undefined : () => targets.vaultUnlock.ask(alias)} />
        )}
        {info.remoteVersion === null ? null : (
          <Notice tone="danger" compact>{t("engineInfo.incompatible", { alias })}</Notice>
        )}
        {info.problem === "" ? null : <Notice tone="danger" compact>{info.problem}</Notice>}
        <div className="flex flex-wrap gap-2">
          {engineActions.map((action) => (
            <Button key={action} kind={action === "update" ? "primary" : action === "stop" ? "danger" : "secondary"} disabled={busy} onClick={() => setConfirming(action)}>
              {info.running === action ? t(actionLabels[action].running) : t(actionLabels[action].button)}
            </Button>
          ))}
        </div>
        {info.actionProblem === "" ? null : <p role="alert" className="text-sm text-danger">{info.actionProblem}</p>}
        {info.lastResult === null ? null : (
          <div className="flex flex-col gap-1">
            <p role="status" className={`text-sm ${info.lastResult.succeeded ? "text-ink" : "text-danger"}`}>
              {info.lastResult.succeeded ? t(actionLabels[info.lastResult.action].done) : t("engineInfo.actionFailed")}
            </p>
            {info.lastResult.output === "" ? null : (
              <pre aria-label={t("engineInfo.output")} className="max-h-40 overflow-auto rounded-md bg-control p-2 font-mono text-[11px] text-ink-muted">
                {info.lastResult.output}
              </pre>
            )}
          </div>
        )}
        <div className="flex justify-end">
          <Button ref={closeRef} disabled={busy} onClick={onClose}>{t("engineInfo.close")}</Button>
        </div>
      </ModalShell>
      {confirming === null ? null : (
        <ConfirmDialog
          id={`${headingId}-confirm`}
          heading={t(actionLabels[confirming].confirm, { alias })}
          body={<p className="text-sm text-ink-muted">{confirmBody(t, confirming, info.status)}</p>}
          confirmLabel={t(actionLabels[confirming].button)}
          cancelLabel={t("engineInfo.cancel")}
          confirmKind={confirming === "stop" ? "danger" : "primary"}
          onConfirm={() => {
            const action = confirming;
            // The dialog closes first; progress and failure show in the
            // information dialog behind it, which stays open.
            setConfirming(null);
            void info.run(action);
          }}
          onCancel={() => setConfirming(null)}
        />
      )}
    </>
  );
}

function EngineFacts({
  status,
  remoteVersion,
  localVersion,
  t,
  onUnlock,
}: {
  status: RemoteEngineStatus | null;
  remoteVersion: string | null;
  localVersion: string;
  t: Translate;
  onUnlock: (() => void) | undefined;
}) {
  const version = status?.version ?? remoteVersion;
  // A different version still connects while the run-target compatibility
  // matches, so only a refused engine shows the difference as a problem.
  const differenceTone = status === null ? "text-danger" : "text-ink-faint";
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
      <dt className="text-ink-faint">{t("engineInfo.version")}</dt>
      <dd className="font-mono text-ink">
        {version}
        {version === localVersion ? null : <span className={`ml-2 text-xs ${differenceTone}`}>{t("engineInfo.versionDiffers", { local: localVersion })}</span>}
      </dd>
      {status === null ? null : <EngineStatusFacts status={status} t={t} onUnlock={onUnlock} />}
    </dl>
  );
}

function EngineStatusFacts({ status, t, onUnlock }: {
  status: RemoteEngineStatus;
  t: Translate;
  onUnlock: (() => void) | undefined;
}) {
  const locked = status.vault && !status.unlocked;
  const vault = !status.vault ? t("engineInfo.vaultNone") : status.unlocked ? t("engineInfo.vaultUnlocked") : t("engineInfo.vaultLocked");
  return (
    <>
      <dt className="text-ink-faint">{t("engineInfo.vault")}</dt>
      <dd className="flex items-center gap-2 text-ink">
        {vault}
        {locked && onUnlock !== undefined ? (
          <button type="button" onClick={onUnlock} className="text-xs text-accent underline-offset-2 hover:underline">
            {t("runTarget.unlockOpen")}
          </button>
        ) : null}
      </dd>
      <dt className="text-ink-faint">{t("engineInfo.sessions")}</dt>
      <dd className="text-ink">{t("engineInfo.sessionCount", { count: String(status.sessions) })}</dd>
    </>
  );
}

// confirmBody says what the action ends: every session, and for a restart or
// an update (which restarts the service), the unlocked Vault.
function confirmBody(t: Translate, action: RemoteEngineAction, status: RemoteEngineStatus | null): string {
  const sessions = status === null
    ? t("engineInfo.stopsSessionsUnknown")
    : status.sessions === 0
      ? t("engineInfo.noSessions", { alias: status.alias })
      : t("engineInfo.stopsSessions", { alias: status.alias, count: String(status.sessions) });
  const locksVault = action !== "stop" && (status === null || (status.vault && !status.passwordless));
  return locksVault ? `${sessions} ${t("engineInfo.locksVault")}` : sessions;
}
