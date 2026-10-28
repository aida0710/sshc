import type { Diagnostic } from "../api/config";
import { useTranslate } from "../i18n/context";
import { ConfigDiagnosticList } from "../sshconfig/ConfigDiagnosticList";
import { Icon } from "../ui/icons";

// ConfigDiagnostics は、Include階層の下に、Include をたどったときに見つかった問題を出す。
// 問題が無ければ、無いことを1行で伝える。列の下端に置くので、スマホでは浮いた下タブの上に空ける。
export function ConfigDiagnostics({ diagnostics }: { diagnostics: Diagnostic[] }) {
  const t = useTranslate();
  if (diagnostics.length === 0) {
    return (
      <p className="mx-3 mb-[calc(0.875rem+var(--mobile-navigation-inset))] flex shrink-0 items-center gap-2 border-t border-line px-2.5 pt-2.5 text-[12.5px] text-ink-faint">
        <Icon name="check" className="size-3.5 shrink-0" />
        {t("explorer.noIncludeProblem")}
      </p>
    );
  }
  return (
    <section aria-labelledby="config-diagnostics-heading" className="mx-3 mb-[calc(0.875rem+var(--mobile-navigation-inset))] flex max-h-56 shrink-0 flex-col border-t border-line px-2.5 pt-2.5">
      <h3 id="config-diagnostics-heading" className="flex items-center gap-2 text-[12.5px] font-medium text-notice-ink">
        <Icon name="warning" className="size-3.5 shrink-0" />
        {t("explorer.diagnosticsCount", { count: diagnostics.length })}
      </h3>
      <ConfigDiagnosticList diagnostics={diagnostics} className="mt-1.5 min-h-0 overflow-y-auto" />
    </section>
  );
}
