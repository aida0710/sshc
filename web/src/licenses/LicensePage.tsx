import { useState } from "react";
import catalogue from "./catalogue.generated.json";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";

export function LicensePage() {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const needle = query.trim().toLocaleLowerCase();
  const entries = catalogue.filter((entry) => `${entry.name} ${entry.version} ${entry.license} ${entry.category}`.toLocaleLowerCase().includes(needle));
  return (
    <section aria-labelledby="license-heading" className="mx-auto flex w-full max-w-5xl flex-col gap-4">
      <header className="border-b border-line pb-3">
        <h2 id="license-heading" className="text-xl font-semibold text-ink">{t("section.license")}</h2>
        <p className="mt-2 text-sm text-ink-muted">{t("license.description")}</p>
      </header>
      <label className="flex flex-col gap-1 text-sm text-ink-muted">
        {t("license.search")}
        <input type="search" className={control} value={query} onChange={(event) => setQuery(event.target.value)} />
      </label>
      <p role="status" className="text-xs text-ink-muted">{t("license.count", { count: entries.length })}</p>
      {entries.length === 0 ? <p className="text-sm text-ink-muted">{t("license.noMatches")}</p> : null}
      <ul className="divide-y divide-line overflow-hidden rounded border border-line bg-card">
        {entries.map((entry) => (
          <li key={`${entry.category}:${entry.name}`}>
            <details className="group">
              <summary className="cursor-pointer px-4 py-3 focus-visible:outline-2 focus-visible:outline-accent">
                <span className="ml-2 break-words text-sm font-medium text-ink">{entry.name}</span>
                <span className="ml-2 text-xs text-ink-muted">{entry.version}</span>
                <span className="ml-3 text-xs text-ink-muted">{entry.license}</span>
              </summary>
              <div className="flex flex-col gap-3 border-t border-line p-4">
                <a href={entry.url} target="_blank" rel="noopener noreferrer" className="self-start break-all text-sm text-accent underline">{entry.url}</a>
                {entry.notices.map((notice) => (
                  <section key={notice.name}>
                    <h3 className="mb-2 text-xs font-medium text-ink-muted">{notice.name}</h3>
                    <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded bg-control p-3 text-xs text-ink" tabIndex={0}>{notice.text}</pre>
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
