import { useId, useState, type ReactNode } from "react";
import { updateApi, type UpdateApi } from "../api/update";
import { useTranslate } from "../i18n/context";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { useUpdateStatus } from "./useUpdateStatus";
import { UpdateControls } from "./UpdateControls";
import { UpdateResultDialog } from "./UpdateResultDialog";
import { isSafeHttpURL } from "../terminal/links";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { explainedUpdateReason, updateMessage } from "./updateMessage";
import { readShownUpdateResult, updateDialogKey, updateJobToShow } from "./updateResultShown";

type UpdateBadgeProps = {
  api?: UpdateApi;
  current?: string;
  indicator?: ReactNode;
  enabled?: boolean;
};

// UpdateBadge shows the version, and under it only what concerns the next release:
// that one is available, or why this installation cannot update from the Web. That
// part folds under the version. An update's progress and result open in a dialog.
export function UpdateBadge({ api = updateApi, current = "", indicator, enabled = true }: UpdateBadgeProps) {
  const t = useTranslate();
  const { status, setStatus } = useUpdateStatus(api, enabled);
  const [shownResultId] = useState(readShownUpdateResult);
  const [closedDialogKey, setClosedDialogKey] = useState<string | null>(null);
  const [foldedRelease, setFoldedRelease] = useState(() => readStoredValue(localStorageKeys.updateFoldedRelease));
  const [isReasonOpen, setReasonOpen] = useState(false);
  const detailsId = useId();

  const displayedCurrent = status?.current ?? current;
  if (displayedCurrent === "") {
    return null;
  }
  const versionLabel = t("update.version", { version: displayedCurrent });
  const reason = status === null ? undefined : explainedUpdateReason(status);
  const release = status?.available === true && status.latest !== undefined ? status.latest : undefined;
  const releasePage = status?.pageUrl !== undefined && isSafeHttpURL(status.pageUrl) ? status.pageUrl : undefined;
  const hasDetails = reason !== undefined || release !== undefined;
  // The notice of a release stays open until it is folded; a newer release unfolds it.
  // The reason only says how to update outside the Web UI, so it starts folded.
  const isOpen = release === undefined ? isReasonOpen : foldedRelease !== release;
  const detailIndent = indicator === undefined ? "mt-1" : "mt-1 pl-3.5";

  function toggleDetails() {
    if (release === undefined) {
      setReasonOpen(!isReasonOpen);
      return;
    }
    const folded = isOpen ? release : "";
    writeStoredValue(localStorageKeys.updateFoldedRelease, folded);
    setFoldedRelease(folded);
  }

  const dialogJob = status === null ? undefined : updateJobToShow(status, shownResultId);
  const openDialogJob = dialogJob !== undefined && updateDialogKey(dialogJob) !== closedDialogKey ? dialogJob : undefined;

  return (
    <div className="border-t border-line px-2 py-2 text-xs text-ink-muted">
      {!hasDetails ? (
        <div className="flex items-center gap-2">
          {indicator}
          <p>{versionLabel}</p>
        </div>
      ) : (
        <button
          type="button"
          aria-expanded={isOpen}
          aria-controls={detailsId}
          className="flex items-center gap-2 text-left hover:text-ink"
          onClick={toggleDetails}
        >
          {indicator}
          <span>{versionLabel}</span>
          <DisclosureChevron expanded={isOpen} className="size-3" />
        </button>
      )}
      <div id={detailsId} hidden={hasDetails && !isOpen}>
        {reason === undefined ? null : <p className={detailIndent}>{t(updateMessage(reason))}</p>}
        {release === undefined || releasePage === undefined ? null : (
          <p className={detailIndent}>
            <a href={releasePage} target="_blank" rel="noreferrer noopener" className="text-ink underline underline-offset-2">
              {t("update.available", { version: release })}
            </a>
          </p>
        )}
        {status === null ? null : (
          <div className={indicator === undefined ? "" : "pl-3.5"}>
            <UpdateControls status={status} api={api} onStart={(job) => setStatus({ ...status, canUpdate: false, job })} />
          </div>
        )}
      </div>
      {openDialogJob === undefined ? null : (
        <UpdateResultDialog job={openDialogJob} onClose={() => setClosedDialogKey(updateDialogKey(openDialogJob))} />
      )}
    </div>
  );
}
