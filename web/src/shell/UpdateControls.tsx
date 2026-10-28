import { useRef, useState } from "react";
import { failureCode } from "../api/client";
import type { TerminalSessionsApi } from "../api/terminalSessions";
import type { UpdateApi, UpdateJob, UpdatePreview, UpdateStatus } from "../api/update";
import { useTranslate } from "../i18n/context";
import { EngineEndingSessionsList } from "../terminal/EndingSessionsList";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button } from "../ui/surface";
import { updateMessage } from "./updateMessage";
import { isUpdateActive } from "./useUpdateStatus";

type UpdateControlsProps = {
  status: UpdateStatus;
  api: UpdateApi;
  // sessions is this machine's terminal API: the update restarts this
  // machine's engine, whatever run target the terminal shows.
  sessions: Pick<TerminalSessionsApi, "terminalSessions">;
  onStart: (job: UpdateJob) => void;
};

export function UpdateControls({ status, api, sessions, onStart }: UpdateControlsProps) {
  const t = useTranslate();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [preview, setPreview] = useState<UpdatePreview | null>(null);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const job = status.job;

  async function review() {
    if (loading || status.latest === undefined) return;
    setError("");
    setPreview(null);
    setOpen(true);
    setLoading(true);
    try {
      setPreview(await api.previewUpdate(status.latest));
    } catch (cause) {
      setError(t(updateMessage(failureCode(cause))));
    } finally {
      setLoading(false);
    }
  }

  async function start() {
    if (preview === null) return;
    setError("");
    try {
      const started = await api.startUpdate(preview.target, preview.actionToken);
      onStart(started);
      setOpen(false);
      setPreview(null);
    } catch (cause) {
      // A consumed or expired token is never retried. Review the current plan again.
      setPreview(null);
      const code = failureCode(cause);
      setError(t(code === "" ? "update.resultUnknown" : updateMessage(code)));
    }
  }

  return (
    <>
      {status.canUpdate !== true || isUpdateActive(job) ? null : (
        <Button ref={buttonRef} kind="primary" disabled={loading} onClick={() => { void review(); }}>
          {t("update.install")}
        </Button>
      )}
      {!open ? null : (
        <ConfirmDialog
          id="self-update-confirm"
          heading={t("update.confirmTitle")}
          body={
            <>
              <p>{preview === null ? t("update.preparing") : t(preview.manager === "homebrew" ? "update.confirmHomebrewVersion" : "update.confirmVersion", {
                current: preview.current,
                target: preview.target,
                manager: preview.manager === "homebrew" ? "Homebrew" : preview.manager,
              })}</p>
              <p>{t("update.disconnectWarning")}</p>
              <EngineEndingSessionsList api={sessions} />
              <p>{t("update.vaultWarning")}</p>
            </>
          }
          confirmLabel={t("update.install")}
          cancelLabel={t("update.cancel")}
          confirmKind="primary"
          confirmDisabled={preview === null}
          error={error}
          returnFocusRef={buttonRef}
          onConfirm={start}
          onCancel={() => { setOpen(false); setPreview(null); }}
        />
      )}
    </>
  );
}
