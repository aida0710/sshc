import type { UpdateStatus } from "../api/update";
import { useTranslate } from "../i18n/context";
import { isReleasePageURL } from "../shell/sshcRepository";
import { installsFromReleasePage } from "../shell/updateMessage";
import { availableRelease } from "../shell/useUpdateStatus";
import { primaryAction } from "../ui/form";

// ReleasePageLink takes an installation that cannot replace itself to the page of the newer release. It stands where
// the update button stands on an installation that can.
export function ReleasePageLink({ status }: { status: UpdateStatus }) {
  const t = useTranslate();
  const { pageUrl } = status;
  if (!installsFromReleasePage(status) || availableRelease(status) === undefined || !isReleasePageURL(pageUrl)) return null;
  return (
    <a href={pageUrl} target="_blank" rel="noopener noreferrer" className={`${primaryAction} inline-flex min-h-11 items-center md:min-h-0`}>
      {t("update.openReleasePage")}
    </a>
  );
}
