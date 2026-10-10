import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import type { SFTPSearchModel } from "./useSFTPSearch";
import { DisclosureSummary } from "../ui/DisclosureSummary";

export function SFTPSearchControls({ search, disabled }: { search: SFTPSearchModel; disabled: boolean }) {
  const t = useTranslate();
  return <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-line/50 px-2 py-1 text-xs text-ink-muted">
    <label className="flex items-center gap-2">
      {t("sftp.search.mode")}
      <select aria-label={t("sftp.search.mode")} value={search.mode} disabled={disabled} onChange={(event) => search.setMode(event.target.value === "content" ? "content" : "name")} className="rounded border border-control-line bg-control px-2 py-1">
        <option value="name">{t("sftp.search.name")}</option>
        <option value="content">{t("sftp.search.content")}</option>
      </select>
    </label>
    {search.mode === "content" ? <details className="min-w-0 flex-1"><DisclosureSummary className="min-h-8 focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent">{t("sftp.search.contentScope")}</DisclosureSummary><p className="pb-2 leading-5">{t("sftp.search.contentHint")}</p></details> : null}
    {search.searching ? <Button onClick={search.cancelSearch}>{t("sftp.search.stop")}</Button> : null}
  </div>;
}
