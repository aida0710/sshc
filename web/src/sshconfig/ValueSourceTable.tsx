import { shownFilePath } from "../android/shownFilePath";
import type { EffectiveResponse } from "../api/diagnostics";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";

type ValueSource = EffectiveResponse["sources"][number];

// A column of its own only once the table has room for it; narrower, its text is in the value's cell.
const onlyWhenWide = "hidden @2xl/sources:table-cell";

// ValueSourceTable lists every line of the SSH configuration that sets a value of a connection, and which of them
// applies. Where the table is narrow (a phone, or the editor beside the list of connections) the place and the state
// go under the value, so that nothing has to be scrolled sideways: a value is often longer than the whole width.
export function ValueSourceTable({ sources, label, captionShown = false, showCondition = false }: {
  sources: readonly ValueSource[];
  label: string;
  captionShown?: boolean;
  // Only a page with room for it shows the Match condition a value sits under.
  showCondition?: boolean;
}) {
  const t = useTranslate();
  return (
    <div className="@container/sources">
      <table className="w-full text-xs">
        <caption className={captionShown ? `mb-2 text-left ${hintText}` : "sr-only"}>{label}</caption>
        <thead>
          <tr className="text-left text-xs text-ink-faint">
            <th scope="col" className="pb-1.5 pr-3 font-normal">{t("diag.columnKeyword")}</th>
            <th scope="col" className="pb-1.5 pr-3 font-normal">{t("diag.columnValue")}</th>
            <th scope="col" className={`${onlyWhenWide} pb-1.5 pr-3 font-normal`}>{t("diag.columnWhere")}</th>
            {showCondition ? <th scope="col" className={`${onlyWhenWide} pb-1.5 pr-3 font-normal`}>{t("diag.columnCondition")}</th> : null}
            <th scope="col" className={`${onlyWhenWide} pb-1.5 font-normal`}>{t("diag.columnState")}</th>
          </tr>
        </thead>
        <tbody>
          {sources.map((source) => {
            const where = `${shownFilePath(source.path)}:${source.line}`;
            const state = t(source.winner ? "diag.inEffect" : "diag.superseded");
            return (
              <tr key={`${source.path}:${source.line}:${source.keyword}`} className={`border-t border-line ${source.winner ? "" : "opacity-60"}`}>
                <th scope="row" className="py-1.5 pr-3 text-left align-top font-mono text-xs font-normal text-ink-muted">{source.keyword}</th>
                <td className="py-1.5 pr-3 align-top font-mono text-xs text-ink">
                  <span className="[overflow-wrap:anywhere]">{source.value}</span>
                  <span className="mt-0.5 block text-xs text-ink-faint [overflow-wrap:anywhere] @2xl/sources:hidden">
                    {where}
                    {showCondition && source.condition !== "" ? ` · ${source.condition}` : ""}
                    {` · ${state}`}
                  </span>
                </td>
                <td className={`${onlyWhenWide} py-1.5 pr-3 align-top font-mono text-xs text-ink-faint [overflow-wrap:anywhere]`}>{where}</td>
                {showCondition ? <td className={`${onlyWhenWide} py-1.5 pr-3 align-top font-mono text-xs text-ink-faint [overflow-wrap:anywhere]`}>{source.condition}</td> : null}
                <td className={`${onlyWhenWide} whitespace-nowrap py-1.5 align-top text-xs text-ink-faint`}>{state}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
