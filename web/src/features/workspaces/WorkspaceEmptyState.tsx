import { useTranslate } from "../../i18n/context";
import { BrandMark } from "../../ui/BrandMark";

// Nothing is open: a nudge towards the first connection.
export function WorkspaceEmptyState() {
  const t = useTranslate();
  return (
    <div className="flex h-full items-center justify-center p-6">
      <section className="w-full max-w-md border-y border-line bg-surface-subtle p-6 text-center" role="status">
        <BrandMark className="mx-auto size-10" />
        <h2 className="mt-4 text-lg font-semibold tracking-tight text-ink">{t("terminal.emptyHeading")}</h2>
        <p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-ink-muted">{t("terminal.emptyHint")}</p>
        <div aria-hidden="true" className="mx-auto mt-4 max-w-xs rounded bg-term-bg px-4 py-3 text-left font-mono text-xs text-ink"><span className="text-live">$</span> sshc host<span className="ml-1 inline-block h-3 w-1.5 translate-y-0.5 bg-ink" /></div>
      </section>
    </div>
  );
}
