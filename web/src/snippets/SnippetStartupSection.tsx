import { useTranslate } from "../i18n/context";
import { FieldRow, FieldRowNote } from "../ui/FieldRow";
import { control } from "../ui/form";
import { PageSection } from "../ui/page";
import { Button, Notice } from "../ui/surface";

// SnippetStartupSection は、接続先のシェルの準備ができるたびに開いているスニペットを送る割り当ての節である。
// 接続先が変わって止まった割り当ては、どのホストのものかを一覧で見せる。
export function SnippetStartupSection({
  aliases,
  alias,
  staleAliases,
  sendsSecrets,
  assigning,
  busy,
  onAliasChange,
  onAssign,
  onClear,
}: {
  aliases: string[];
  alias: string;
  staleAliases: string[];
  // sendsSecrets は、スニペットにシークレットの変数があるかである。あるときは値が残ることを知らせる。
  sendsSecrets: boolean;
  assigning: boolean;
  // busy は、このページのどれかの操作を待っている間である。終わるまでボタンを止める。
  busy: boolean;
  onAliasChange: (alias: string) => void;
  onAssign: () => void;
  onClear: () => void;
}) {
  const t = useTranslate();
  return (
    <PageSection heading={t("snippets.startup")}>
      <div>
        <FieldRow
          label={t("snippets.startupTarget")}
          hint={t(sendsSecrets ? "snippets.startupHintWithSecret" : "snippets.startupHint")}
          warning={staleAliases.includes(alias) ? t("snippets.startupStale") : undefined}
          interactiveChildren
        >
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <select
              aria-label={t("snippets.startupHost")}
              value={alias}
              onChange={(event) => onAliasChange(event.target.value)}
              className={`${control} sm:w-[220px]`}
            >
              {aliases.map((candidate) => (
                <option key={candidate}>{candidate}</option>
              ))}
            </select>
            <Button busy={assigning} disabled={busy || alias === ""} onClick={onAssign}>
              {t("snippets.setStartup")}
            </Button>
            <button
              type="button"
              disabled={busy || alias === ""}
              className="rounded-md px-2 py-1 text-xs text-ink-muted transition-colors duration-150 hover:text-ink"
              onClick={onClear}
            >
              {t("snippets.clearStartup")}
            </button>
          </div>
        </FieldRow>
        {staleAliases.length === 0 ? null : (
          <FieldRowNote>
            <Notice compact>{t("snippets.startupStaleCount", { count: staleAliases.length })}</Notice>
            <ul aria-label={t("snippets.startupStaleHosts")} className="flex flex-wrap gap-1">
              {staleAliases.map((stale) => (
                <li key={stale} className="rounded border border-line px-1.5 py-0.5 font-mono text-xs text-ink-muted">
                  {stale}
                </li>
              ))}
            </ul>
          </FieldRowNote>
        )}
      </div>
    </PageSection>
  );
}
