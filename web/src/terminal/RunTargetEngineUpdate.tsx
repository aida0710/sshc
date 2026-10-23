import { useEffect, type ReactNode } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useTranslate } from "../i18n/context";
import { CommandOutput } from "../ui/CommandOutput";
import { Spinner } from "../ui/Spinner";
import { Button } from "../ui/surface";
import { useElapsedSeconds } from "../ui/useElapsedSeconds";
import { useEngineInfo } from "./useEngineInfo";

// RunTargetEngineUpdate shows the two sshc versions that do not work together
// and updates the run target's sshc in the terminal area itself. Unlike the
// engine information dialog, it does not ask for a confirmation: what the
// update ends is said next to the button before it is pressed, and nothing on
// that engine can be used from here until it is updated anyway.
export function RunTargetEngineUpdate({
  alias,
  api,
  remoteVersion,
  localVersion,
  vaultRevision,
  onUpdated,
  children,
}: {
  alias: string;
  api: RemoteEnginesApi;
  // remoteVersion is the version the engine was refused with.
  remoteVersion: string;
  localVersion: string;
  vaultRevision: number;
  // onUpdated reads the session list again once the host's sshc has ended
  // and succeeded.
  onUpdated: () => void;
  // children are the panel's other actions, shown beside the update.
  children: ReactNode;
}) {
  const t = useTranslate();
  const info = useEngineInfo({ alias, api, translate: t, vaultRevision });
  const updating = info.running === "update";
  const elapsed = useElapsedSeconds(updating);
  const succeeded = info.lastResult?.succeeded === true;

  useEffect(() => { if (succeeded) onUpdated(); }, [succeeded, onUpdated]);

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2">
        <VersionCard label={alias} version={remoteVersion} differs />
        <span aria-hidden="true" className="text-ink-faint">→</span>
        <VersionCard label={t("runTarget.thisMachineNote")} version={localVersion} differs={false} />
      </div>
      {updating ? (
        <p role="status" className="flex items-center gap-2 text-xs text-ink-muted">
          <Spinner />
          {t("runTarget.updateProgress", { alias, seconds: String(elapsed) })}
        </p>
      ) : null}
      {info.actionProblem === "" ? null : <p role="alert" className="break-words text-sm text-danger">{info.actionProblem}</p>}
      {info.lastResult === null ? null : succeeded
        ? <p role="status" className="text-sm text-ink">{t("engineInfo.updated")}</p>
        : <p role="alert" className="text-sm text-danger">{t("engineInfo.actionFailed")}</p>}
      <CommandOutput output={info.output} label={t("engineInfo.output")} />
      <div className="flex flex-wrap gap-2">
        <Button kind="primary" busy={updating} disabled={info.running !== null} onClick={() => void info.run("update")}>
          {updating ? t("runTarget.updating", { alias }) : t("runTarget.update", { alias })}
        </Button>
        {children}
      </div>
      <p className="text-xs text-ink-muted">{t("runTarget.updateConsequence", { alias })}</p>
    </div>
  );
}

function VersionCard({ label, version, differs }: { label: string; version: string; differs: boolean }) {
  return (
    <div className="min-w-0 rounded-md border border-line px-3 py-2">
      <p className="truncate text-xs text-ink-muted">{label}</p>
      <p className={`truncate font-mono text-sm ${differs ? "text-danger" : "text-ink"}`}>{version}</p>
    </div>
  );
}
