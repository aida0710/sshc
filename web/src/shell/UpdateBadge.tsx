import { sectionPath } from "../routing/sectionRoute";
import { useId, useState, type MouseEvent, type ReactNode } from "react";
import { updateApi, type UpdateApi } from "../api/update";
import { useTranslate } from "../i18n/context";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { isUpdateActive, useUpdateStatus } from "./useUpdateStatus";
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
  // About lists an update's progress itself; while it is open, only the result opens a dialog.
  showsProgressDialog?: boolean;
  onNavigate?: (event: MouseEvent<HTMLAnchorElement>) => void;
};

// UpdateBadge shows the version, and under it only what concerns the next release:
// that one is available, or why this installation cannot update from the Web. That
// part has its own disclosure button beside the version link. An update is confirmed
// and followed on About, so the notice links there; its result opens in a dialog.
export function UpdateBadge({ api = updateApi, current = "", indicator, enabled = true, showsProgressDialog = true, onNavigate }: UpdateBadgeProps) {
  const t = useTranslate();
  const { status } = useUpdateStatus(api, enabled);
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

  function followAbout(event: MouseEvent<HTMLAnchorElement>) {
    if (onNavigate === undefined || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    onNavigate(event);
  }

  const dialogJob = status === null ? undefined : updateJobToShow(status, shownResultId);
  const isProgressOnPage = !showsProgressDialog && isUpdateActive(dialogJob);
  const openDialogJob = dialogJob !== undefined && !isProgressOnPage && updateDialogKey(dialogJob) !== closedDialogKey ? dialogJob : undefined;
  const offersUpdate = status?.canUpdate === true && !isUpdateActive(status.job);

  return (
    <div className="border-t border-line px-2 py-2 text-xs text-ink-muted">
      <div className="flex items-center gap-2">
        {indicator}
        <a href={sectionPath("About")} onClick={followAbout} className="hover:text-ink hover:underline">{versionLabel}</a>
        {!hasDetails ? null : (
          <button type="button" aria-label={t("update.toggleNotice")} aria-expanded={isOpen}
            aria-controls={detailsId} className="rounded p-1 hover:bg-hover hover:text-ink" onClick={toggleDetails}>
            <DisclosureChevron expanded={isOpen} className="size-3" />
          </button>
        )}
      </div>
      <div id={detailsId} hidden={hasDetails && !isOpen}>
        {reason === undefined ? null : <p className={detailIndent}>{t(updateMessage(reason))}</p>}
        {release === undefined || releasePage === undefined ? null : (
          <p className={detailIndent}>
            <a href={releasePage} target="_blank" rel="noreferrer noopener" className="text-ink underline underline-offset-2">
              {t("update.available", { version: release })}
            </a>
          </p>
        )}
        {!offersUpdate ? null : (
          <p className={detailIndent}>
            <a href={sectionPath("About")} onClick={followAbout} className="text-ink underline underline-offset-2">{t("update.openAbout")}</a>
          </p>
        )}
      </div>
      {openDialogJob === undefined ? null : (
        <UpdateResultDialog job={openDialogJob} onClose={() => setClosedDialogKey(updateDialogKey(openDialogJob))} />
      )}
    </div>
  );
}
