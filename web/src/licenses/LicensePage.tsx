import { useState } from "react";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import catalogue from "./catalogue.generated.json";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { PageHeader } from "../ui/page";

export function LicensePage() {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const needle = query.trim().toLocaleLowerCase();
  const entries = catalogue.filter((entry) => `${entry.name} ${entry.version} ${entry.license} ${entry.category}`.toLocaleLowerCase().includes(needle));
  return (
    <section aria-labelledby="license-heading" className="flex flex-col gap-4">
      <PageHeader title={t("section.license")} headingId="license-heading" description={t("license.description")} />
      <label className="flex flex-col gap-1 text-sm text-ink-muted">
        {t("license.search")}
        <input type="search" className={control} value={query} onChange={(event) => setQuery(event.target.value)} />
      </label>
      <p role="status" className="text-xs text-ink-muted">{t("license.count", { count: entries.length })}</p>
      {entries.length === 0 ? <p className="text-sm text-ink-muted">{t("license.noMatches")}</p> : null}
      <ul className="divide-y divide-line border-y border-line">
        {entries.map((entry) => (
          <li key={`${entry.category}:${entry.name}`}>
            <details className="group">
              <DisclosureSummary className="flex-wrap py-3 focus-visible:outline-2 focus-visible:outline-accent">
                <span className="ml-0.5 break-words text-sm font-medium text-ink">{entry.name}</span>
                <span className="ml-0.5 text-xs text-ink-muted">{entry.version}</span>
                <span className="ml-1.5 text-xs text-ink-muted">{entry.license}</span>
              </DisclosureSummary>
              <div className="flex flex-col gap-3 pb-4 pl-6">
                <a href={entry.url} target="_blank" rel="noopener noreferrer" className="self-start break-all text-sm text-accent underline">{entry.url}</a>
                {entry.notices.map((notice) => (
                  <section key={notice.name}>
                    <h3 className="mb-2 text-xs font-medium text-ink-muted">{notice.name}</h3>
                    <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md bg-well p-3 text-xs text-ink" tabIndex={0}>{notice.text}</pre>
                  </section>
                ))}
              </div>
            </details>
          </li>
        ))}
      </ul>
    </section>
  );
}
