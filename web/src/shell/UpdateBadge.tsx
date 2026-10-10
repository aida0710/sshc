import { useId, useState, type ReactNode } from "react";
import { updateApi, type UpdateApi } from "../api/update";
import { useTranslate } from "../i18n/context";
import { useUpdateStatus } from "./useUpdateStatus";
import { UpdateControls } from "./UpdateControls";
import { isSafeHttpURL } from "../terminal/links";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { explainedUpdateReason, updateMessage } from "./updateMessage";

type UpdateBadgeProps = {
  api?: UpdateApi;
  current?: string;
  indicator?: ReactNode;
  enabled?: boolean;
};

export function UpdateBadge({ api = updateApi, current = "", indicator, enabled = true }: UpdateBadgeProps) {
  const t = useTranslate();
  const { status, setStatus } = useUpdateStatus(api, enabled);
  const [isReasonOpen, setReasonOpen] = useState(false);
  const reasonId = useId();

  const displayedCurrent = status?.current ?? current;
  if (displayedCurrent === "") {
    return null;
  }
  const versionLabel = t("update.version", { version: displayedCurrent });
  const reason = status === null ? undefined : explainedUpdateReason(status);
  const detailIndent = indicator === undefined ? "mt-1" : "mt-1 pl-3.5";
  return (
    <div className="border-t border-line px-2 py-2 text-xs text-ink-muted">
      {reason === undefined ? (
        <div className="flex items-center gap-2">
          {indicator}
          <p>{versionLabel}</p>
        </div>
      ) : (
        // The reason only says how to update outside the Web UI, so it stays folded under the version.
        <button
          type="button"
          aria-expanded={isReasonOpen}
          aria-controls={reasonId}
          className="flex items-center gap-2 text-left hover:text-ink"
          onClick={() => setReasonOpen(!isReasonOpen)}
        >
          {indicator}
          <span>{versionLabel}</span>
          <DisclosureChevron expanded={isReasonOpen} className="size-3" />
        </button>
      )}
      {reason === undefined ? null : (
        <p id={reasonId} hidden={!isReasonOpen} className={detailIndent}>{t(updateMessage(reason))}</p>
      )}
      {status === null || !status.available || status.pageUrl === undefined || !isSafeHttpURL(status.pageUrl) ? null : (
        <p className={detailIndent}>
          <a
            href={status.pageUrl}
            target="_blank"
            rel="noreferrer noopener"
            className="text-ink underline underline-offset-2"
          >
            {t("update.available", { version: status.latest ?? "" })}
          </a>
        </p>
      )}
      {status === null ? null : <UpdateControls status={status} api={api} onStart={(job) => setStatus({ ...status, canUpdate: false, job })} />}
    </div>
  );
}
