import type { UpdateStatus } from "../api/update";
import { useLanguage } from "../i18n/context";
import { isSafeHttpURL } from "../terminal/links";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { formatDateTime } from "../ui/format";
import { PageSection } from "../ui/page";

type ReleaseNotesProps = {
  releases: NonNullable<UpdateStatus["releases"]>;
  current: string;
  latest: string | undefined;
};

export function ReleaseNotes({ releases, current, latest }: ReleaseNotesProps) {
  const { t, locale } = useLanguage();
  return (
    <PageSection heading={t("about.releaseNotes")} headingId="about-notes-heading">
      {releases.length === 0 ? <p className="text-sm text-ink-muted">{t("about.noNotes")}</p> : (
        <div className="divide-y divide-line">
          {releases.map((release) => {
            const isCurrent = release.version.replace(/^v/, "") === current.replace(/^v/, "");
            return (
              <details key={release.version} open={release.version === latest && !isCurrent} className="group">
                <DisclosureSummary className="flex-wrap py-3">
                  <span className="font-medium text-ink">{release.version}</span>
                  {isCurrent ? <span className="text-xs text-ink-muted">{t("about.currentRelease")}</span> : null}
                  {release.publishedAt === undefined ? null : <time dateTime={release.publishedAt} className="text-xs text-ink-muted">{formatDateTime(release.publishedAt, locale)}</time>}
                </DisclosureSummary>
                <div className="grid gap-3 pb-4 pl-6 text-sm">
                  <div className="whitespace-pre-wrap [overflow-wrap:anywhere] text-ink">{release.body || t("about.emptyNotes")}</div>
                  {!isSafeHttpURL(release.pageUrl) ? null : <a href={release.pageUrl} target="_blank" rel="noopener noreferrer" className="justify-self-start text-accent underline">{t("about.viewRelease")}</a>}
                </div>
              </details>
            );
          })}
        </div>
      )}
    </PageSection>
  );
}
