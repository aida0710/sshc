import { useTranslate } from "../i18n/context";
import { BrandMark } from "../ui/BrandMark";
import { PageBoard } from "../ui/PageBoard";

// Nothing is open: a nudge towards the first connection, in the same card as the pages and the terminal.
export function TerminalEmptyState() {
  const t = useTranslate();
  return (
    // The card is not named, so it does not become a region beside the panels that explain a refused run target.
    <PageBoard fill className="items-center justify-center p-6">
      <div className="flex max-w-md flex-col items-center text-center" role="status">
        <BrandMark className="size-10" />
        <h2 className="mt-4 text-lg font-semibold text-ink">{t("terminal.emptyHeading")}</h2>
        <p className="mt-1.5 max-w-sm text-[13px] leading-5 text-ink-muted">{t("terminal.emptyHint")}</p>
        <div aria-hidden="true" className="mt-4 w-full max-w-xs rounded-md bg-term-bg px-4 py-3 text-left font-mono text-xs text-ink"><span className="text-live">$</span> sshc host<span className="ml-1 inline-block h-3 w-1.5 translate-y-0.5 bg-ink" /></div>
      </div>
    </PageBoard>
  );
}
