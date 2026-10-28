import { dialogHeading } from "../ui/form";
import { useEffect, useId, useRef } from "react";
import type { UpdateJob } from "../api/update";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { updateJobMessage } from "./updateMessage";
import { rememberShownUpdateResult } from "./updateResultShown";
import { isUpdateActive } from "./useUpdateStatus";

export function UpdateResultDialog({ job, onClose }: { job: UpdateJob; onClose: () => void }) {
  const t = useTranslate();
  const headingId = useId();
  const bodyId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const finished = !isUpdateActive(job);

  useEffect(() => {
    if (finished) rememberShownUpdateResult(job.id);
  }, [finished, job.id]);

  return (
    <ModalShell
      labelledBy={headingId}
      describedBy={bodyId}
      onDismiss={onClose}
      initialFocusRef={closeRef}
      panelClassName="flex w-full max-w-sm flex-col gap-3 rounded-lg p-4"
    >
      <h2 id={headingId} className={dialogHeading}>{t("update.resultTitle")}</h2>
      <p id={bodyId} role="status" className="text-sm text-ink-muted">
        {t(updateJobMessage(job), { version: job.installedVersion ?? job.target })}
      </p>
      <div className="flex justify-end gap-2">
        {job.state === "restarting" || job.state === "restart_required" ? (
          <Button onClick={() => window.location.reload()}>{t("update.reload")}</Button>
        ) : null}
        <Button ref={closeRef} kind="primary" onClick={onClose}>{t("update.close")}</Button>
      </div>
    </ModalShell>
  );
}
