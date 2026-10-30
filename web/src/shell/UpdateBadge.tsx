import { sectionPath } from "../routing/sectionRoute";
import { opensInThisPage } from "../ui/linkClick";
import { useId, useState, type MouseEvent, type ReactNode } from "react";
import { updateApi, type UpdateApi } from "../api/update";
import { useTranslate } from "../i18n/context";
import { updateGlobalSettings, useSettings } from "../settings/settingsStore";
import { availableRelease, isUpdateActive, useUpdateStatus } from "./useUpdateStatus";
import { UpdateResultDialog } from "./UpdateResultDialog";
import { isSafeHttpURL } from "../terminal/links";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { explainedUpdateReason, updateMessage } from "./updateMessage";
import { readShownUpdateResult, updateDialogKey, updateJobToShow } from "./updateResultShown";

type UpdateBadgeProps = {
  api?: UpdateApi;
  current?: string;
  // leading is drawn before the version in the same row: the sidebar puts the
  // brand there, so the version reads "sshc | v0.52.1".
  leading?: ReactNode;
  indicator?: ReactNode;
  enabled?: boolean;
  // About lists an update's progress itself; while it is open, only the result opens a dialog.
  showsProgressDialog?: boolean;
  onNavigate?: (event: MouseEvent<HTMLAnchorElement>) => void;
};

// UpdateBadge shows the version in the sidebar heading, and under it only what concerns the next release:
// that one is available, or why this installation cannot update from the Web. That
// part has its own disclosure button beside the version link. An update is confirmed
// and followed on About, so the notice links there; its result opens in a dialog.
export function UpdateBadge({ api = updateApi, current = "", leading, indicator, enabled = true, showsProgressDialog = true, onNavigate }: UpdateBadgeProps) {
  const t = useTranslate();
  const { status } = useUpdateStatus(api, enabled);
  const [shownResultId] = useState(readShownUpdateResult);
  const [closedDialogKey, setClosedDialogKey] = useState<string | null>(null);
  const foldedRelease = useSettings().document?.global.updateNoticeFoldedRelease ?? "";
  const [isReasonOpen, setReasonOpen] = useState(false);
  const detailsId = useId();

  const displayedCurrent = status?.current ?? current;
  if (displayedCurrent === "" && leading === undefined) {
    return null;
  }
  const versionLabel = t("update.version", { version: displayedCurrent });
  const reason = status === null ? undefined : explainedUpdateReason(status);
  const release = availableRelease(status);
  const releasePage = status?.pageUrl !== undefined && isSafeHttpURL(status.pageUrl) ? status.pageUrl : undefined;
  const hasDetails = reason !== undefined || release !== undefined;
  // The notice of a release stays open until it is folded; a newer release unfolds it.
  // The reason only says how to update outside the Web UI, so it starts folded.
  const isOpen = release === undefined ? isReasonOpen : foldedRelease !== release;
  const detailIndent = "mt-1";

  function toggleDetails() {
    if (release === undefined) {
      setReasonOpen(!isReasonOpen);
      return;
    }
    // The fold is kept for every machine, so a release folded on one is folded on the others.
    void updateGlobalSettings(({ updateNoticeFoldedRelease: _unfolded, ...settings }) =>
      isOpen ? { ...settings, updateNoticeFoldedRelease: release } : settings).catch(() => undefined);
  }

  function followAbout(event: MouseEvent<HTMLAnchorElement>) {
    if (onNavigate === undefined || !opensInThisPage(event)) return;
    onNavigate(event);
  }

  const dialogJob = status === null ? undefined : updateJobToShow(status, shownResultId);
  const isProgressOnPage = !showsProgressDialog && isUpdateActive(dialogJob);
  const openDialogJob = dialogJob !== undefined && !isProgressOnPage && updateDialogKey(dialogJob) !== closedDialogKey ? dialogJob : undefined;
  const offersUpdate = status?.canUpdate === true && !isUpdateActive(status.job);

  return (
    <div className="text-xs text-ink-faint">
      <div className="flex h-11 min-w-0 items-center gap-2 pl-1.5 pr-1 md:h-8">
        {leading}
        {displayedCurrent === "" ? null : (
          <>
            {indicator}
            {/* A phone stretches links in the sidebar to 44px touch targets, so the text is centred in the link. */}
            <a href={sectionPath("About")} onClick={followAbout} aria-label={versionLabel} className="flex min-w-0 items-center font-mono hover:text-ink hover:underline"><span className="truncate">{displayedCurrent}</span></a>
          </>
        )}
        {!hasDetails ? null : (
          <button type="button" aria-label={t("update.toggleNotice")} aria-expanded={isOpen}
            aria-controls={detailsId} className="rounded p-1 hover:bg-hover hover:text-ink" onClick={toggleDetails}>
            <DisclosureChevron expanded={isOpen} className="size-3" />
          </button>
        )}
      </div>
      <div id={detailsId} hidden={hasDetails && !isOpen} className="px-1 pb-1">
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
