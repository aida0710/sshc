import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import type { SFTPSearchModel } from "./useSFTPSearch";

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
    {search.mode === "content" ? <p className="min-w-0 flex-1">{t("sftp.search.contentHint")}</p> : null}
    {search.searching ? <Button onClick={search.cancelSearch}>{t("sftp.search.stop")}</Button> : null}
  </div>;
}
