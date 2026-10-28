import { useEffect, useState } from "react";
import type { HostDetail } from "../api/config";
import type { DiagnosticsApi, EffectiveResponse } from "../api/diagnostics";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { EditorSection } from "./EditorSection";
import { NoticeList } from "./NoticeList";
import { useAsyncOperation } from "../ui/useAsyncOperation";

type ConnectionAnalysisProps = {
  detail: HostDetail;
  alias: string;
  api: Pick<DiagnosticsApi, "effective">;
  disabled?: boolean;
};

// ConnectionAnalysis は、接続エディタの「最終的な値」のタブである。保存済みの設定から求めた値と読んだ場所を並べ、
// 押したときだけ、関係するすべての設定行（採用されたかどうかも）を sshc エンジンに尋ねる。
export function ConnectionAnalysis({ detail, alias, api, disabled = false }: ConnectionAnalysisProps) {
  const t = useTranslate();
  const [effective, setEffective] = useState<EffectiveResponse | null>(null);
  const inspection = useAsyncOperation();
  const { clearError } = inspection;

  useEffect(() => {
    setEffective(null);
    clearError();
  }, [alias, detail.file.contents, clearError]);

  async function inspect() {
    if (inspection.busy || disabled) return;
    await inspection.run(() => api.effective(alias), { apply: setEffective, describe: () => t("diag.explainFailed") });
  }

  return (
    <section aria-label={t("conn.analysisLabel")} className="flex flex-col">
      <EditorSection title={t("conn.analysisExplained")}>
        <p className={`-mt-1 mb-2 ${hintText} text-ink-faint`}>{t("conn.analysisExplainedHint")}</p>
        <table className="w-full table-fixed text-[12.5px]">
          <tbody>
            {detail.effective.entries.map((entry, index) => (
              <tr key={`${entry.keyword}-${index}`} className="border-t border-line first:border-t-0">
                <th scope="row" className="w-[36%] truncate py-1.5 pr-2 text-left font-mono text-xs font-normal text-ink-muted">{entry.keyword}</th>
                <td className="truncate py-1.5 pr-2 font-mono text-xs text-ink" title={entry.values.join(" ")}>{entry.values.join(" ")}</td>
                <td className="w-[38%] truncate py-1.5 text-right font-mono text-[11.5px] text-ink-faint" title={sourceText(entry.source)}>{sourceText(entry.source)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <NoticeList notices={detail.effective.notices ?? []} />
      </EditorSection>

      <EditorSection title={t("conn.analysisAuthoritative")}>
        <div className="flex flex-col gap-3">
          <Button className="self-start" disabled={inspection.busy || disabled} onClick={() => void inspect()}>
            {inspection.busy ? t("conn.analysisRunning") : t("conn.analysisRun")}
          </Button>
          {inspection.error === "" ? null : <Notice tone="danger">{inspection.error}</Notice>}

          {effective !== null && effective.executableDirectives.length > 0 ? (
            <div className="rounded-[9px] bg-notice px-3 py-2.5 text-[12.5px]">
              <h4 className="font-medium text-notice-ink">{t("conn.analysisExecutableHeading")}</h4>
              <p className="text-xs text-notice-ink">{t("conn.analysisTokenWarning")}</p>
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
              <table aria-label={t("conn.analysisSources")} className="w-full text-[12.5px]">
                <thead>
                  <tr className="text-left text-[11.5px] text-ink-faint">
                    <th scope="col" className="pb-1.5 pr-3 font-normal">{t("diag.columnKeyword")}</th>
                    <th scope="col" className="pb-1.5 pr-3 font-normal">{t("diag.columnValue")}</th>
                    <th scope="col" className="pb-1.5 pr-3 font-normal">{t("diag.columnWhere")}</th>
                    <th scope="col" className="pb-1.5 font-normal">{t("diag.columnState")}</th>
                  </tr>
                </thead>
                <tbody>
                  {effective.sources.map((source) => (
                    <tr key={`${source.path}:${source.line}:${source.keyword}`} className="border-t border-line">
                      <th scope="row" className="py-1.5 pr-3 text-left font-mono text-xs font-normal text-ink-muted">{source.keyword}</th>
                      <td className="py-1.5 pr-3 font-mono text-xs text-ink">{source.value}</td>
                      <td className="py-1.5 pr-3 font-mono text-[11.5px] text-ink-faint">{`${source.path}:${source.line}`}</td>
                      <td className="py-1.5 text-xs text-ink-faint">{t(source.winner ? "diag.inEffect" : "diag.superseded")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </div>
      </EditorSection>
    </section>
  );
}

// sourceText は、値を読んだ場所（~/.ssh からのファイルと行）である。
function sourceText(source: { path?: string; absolute?: string; line?: number }): string {
  return `${source.path ?? source.absolute ?? ""}:${source.line ?? 0}`;
}
