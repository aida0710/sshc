import { useTranslate } from "../i18n/context";
import type { SFTPSearchModel } from "./useSFTPSearch";
import { DisclosureSummary } from "../ui/DisclosureSummary";

export function SFTPSearchControls({ search, disabled, mobile = false }: { search: SFTPSearchModel; disabled: boolean; mobile?: boolean }) {
  const t = useTranslate();
  return <div className={`flex shrink-0 flex-col items-stretch gap-3 py-2 text-ink-muted ${mobile ? "text-sm" : "text-xs"}`}>
    <label className="flex items-center justify-between gap-2">
      {t("sftp.search.mode")}
      <select aria-label={t("sftp.search.mode")} value={search.mode} disabled={disabled} onChange={(event) => search.setMode(event.target.value === "content" ? "content" : "name")} className={`rounded border border-control-line bg-control px-2 py-1 ${mobile ? "min-h-12 text-base" : ""}`}>
        <option value="name">{t("sftp.search.name")}</option>
        <option value="content">{t("sftp.search.content")}</option>
      </select>
    </label>
    {search.mode === "content" ? <details className="min-w-0 flex-1"><DisclosureSummary className={`${mobile ? "min-h-12 px-2" : "min-h-8"} focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent`}>{t("sftp.search.contentScope")}</DisclosureSummary><p className="pb-2 leading-5">{t("sftp.search.contentHint")}</p></details> : null}
  </div>;
}
