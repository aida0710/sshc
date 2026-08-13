import { useEffect, useState } from "react";
import type { HostDetail } from "../api/config";
import type { EffectiveResponse, IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { hintText, sectionHeading, tableHeadCell, tableHeadRow } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { NoticeList } from "./SavePreview";

type ConnectionAnalysisProps = {
  detail: HostDetail;
  alias: string;
  api: Pick<IntegrationsApi, "effective">;
  disabled?: boolean;
};

export function ConnectionAnalysis({ detail, alias, api, disabled = false }: ConnectionAnalysisProps) {
  const t = useTranslate();
  const [effective, setEffective] = useState<EffectiveResponse | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setEffective(null);
    setError("");
    setBusy(false);
  }, [alias, detail.file.contents]);

  async function inspect() {
    if (busy || disabled) return;
    setBusy(true);
    setError("");
    try {
      setEffective(await api.effective(alias));
    } catch {
      setError(t("diag.explainFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-label={t("conn.analysisLabel")} className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h3 className={sectionHeading}>{t("conn.analysisExplained")}</h3>
        <p className={hintText}>{t("conn.analysisExplainedHint")}</p>
        <ul className="overflow-hidden rounded-xl border border-line bg-card">
          {detail.effective.entries.map((entry, index) => (
            <li key={`${entry.keyword}-${index}`} className="border-t border-hairline px-3 py-2 first:border-t-0">
              <p className="font-mono text-xs text-ink">{`${entry.keyword} ${entry.values.join(" ")}`}</p>
              <p className={hintText}>{`${entry.source.path ?? entry.source.absolute ?? ""}:${entry.source.line ?? 0}`}</p>
            </li>
          ))}
        </ul>
        <NoticeList notices={detail.effective.notices ?? []} />
      </div>

      <div className="flex flex-col gap-3 border-t border-line pt-4">
        <div>
          <h3 className={sectionHeading}>{t("conn.analysisAuthoritative")}</h3>
          <p className={hintText}>{t("conn.analysisAuthoritativeHint")}</p>
        </div>
        <Button className="self-start" disabled={busy || disabled} onClick={() => void inspect()}>
          {busy ? t("conn.analysisRunning") : t("conn.analysisRun")}
        </Button>
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}

        {effective !== null && effective.executableDirectives.length > 0 ? (
          <div className="rounded border border-notice-line bg-notice p-3 text-sm">
            <h4 className="font-medium text-notice-ink">{t("conn.analysisExecutableHeading")}</h4>
            <p className={hintText}>{effective.tokenWarning}</p>
            <ul className="mt-2 flex flex-col gap-2">
              {effective.executableDirectives.map((directive) => (
                <li key={`${directive.path}:${directive.line}:${directive.keyword}`}>
                  <p className="text-ink-muted">
                    {t("conn.checksDirectiveAt", {
                      keyword: directive.keyword,
                      path: directive.path,
                      line: directive.line,
                    })}
                  </p>
                  <pre className="whitespace-pre-wrap break-all text-xs text-ink">{directive.command}</pre>
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        {effective !== null && effective.sources.length > 0 ? (
          <div className="overflow-x-auto">
            <table aria-label={t("conn.analysisSources")} className="w-full text-sm">
              <thead>
                <tr className={tableHeadRow}>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnKeyword")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnValue")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnWhere")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnState")}</th>
                </tr>
              </thead>
              <tbody>
                {effective.sources.map((source) => (
                  <tr key={`${source.path}:${source.line}:${source.keyword}`} className="border-b border-line">
                    <th scope="row" className="py-1.5 pr-3 text-left font-normal text-ink-muted">{source.keyword}</th>
                    <td className="py-1.5 pr-3 font-mono text-xs text-ink">{source.value}</td>
                    <td className="py-1.5 pr-3 font-mono text-xs text-ink-faint">{`${source.path}:${source.line}`}</td>
                    <td className="py-1.5 text-ink-faint">{t(source.winner ? "diag.inEffect" : "diag.superseded")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>
    </section>
  );
}
