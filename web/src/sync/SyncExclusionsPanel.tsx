import { useMemo, useState } from "react";
import { failureCode } from "../api/client";
import type {
  IntegrationsApi,
  SyncExclusions,
} from "../api/integrations";
import { useLanguage } from "../i18n/context";
import { control, hintText } from "../ui/form";
import { Button, Notice } from "../ui/surface";

type Props = {
  api: IntegrationsApi;
  initial?: SyncExclusions;
  onSaved?: (value: SyncExclusions) => void;
};

function exactPattern(path: string): string {
  const escaped = path.replace(/([\\*?\[\]#!])/g, "\\$1");
  return `/${escaped}`;
}

function appendRule(document: string, rule: string): string {
  if (document === "") return `${rule}\n`;
  return `${document}${document.endsWith("\n") ? "" : "\n"}${rule}\n`;
}

export function SyncExclusionsPanel({ api, initial, onSaved }: Props) {
  const { t } = useLanguage();
  const [view, setView] = useState<SyncExclusions | null>(initial ?? null);
  const [document, setDocument] = useState(initial?.document ?? "");
  const [loaded, setLoaded] = useState(initial !== undefined);
  const [search, setSearch] = useState("");
  const [overrides, setOverrides] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    if (loaded || busy) return;
    setBusy(true);
    setError("");
    try {
      const next = await api.syncExclusions();
      setView(next);
      setDocument(next.document);
      setLoaded(true);
    } catch {
      setError(t("sync.exclusions.loadFailed"));
    } finally {
      setBusy(false);
    }
  }

  const candidates = useMemo(() => {
    const normalized = search.trim().toLocaleLowerCase();
    return (view?.candidates ?? []).filter(
      (candidate) =>
        normalized === "" ||
        candidate.path.toLocaleLowerCase().includes(normalized),
    );
  }, [search, view]);
  const ignoredCount = (view?.candidates ?? []).reduce(
    (count, candidate) =>
      count + (overrides[candidate.path] ?? candidate.ignored ? 1 : 0),
    0,
  );
  const sensitiveIgnored = (view?.candidates ?? []).some((candidate) => {
    const ignored = overrides[candidate.path] ?? candidate.ignored;
    return (
      ignored &&
      (candidate.path === "config" ||
        candidate.path === "known_hosts" ||
        candidate.path.startsWith("connections/") ||
        candidate.path.startsWith("keys/"))
    );
  });

  return (
    <details
      className="group overflow-hidden rounded-md border border-control-line bg-card"
      onToggle={(event) => {
        if (event.currentTarget.open) void load();
      }}
    >
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 bg-toolbar px-4 py-3 marker:hidden hover:bg-select-fill">
        <span className="flex min-w-0 items-center gap-3 text-sm font-medium text-ink">
          <span
            aria-hidden="true"
            className="inline-flex size-5 shrink-0 items-center justify-center text-base text-ink-muted transition-transform group-open:rotate-90"
          >
            ›
          </span>
          {t("sync.exclusions.heading")}
        </span>
        <span className={hintText}>
          {loaded
            ? t("sync.exclusions.summary", {
                included: (view?.candidates.length ?? 0) - ignoredCount,
                ignored: ignoredCount,
              })
            : t("sync.exclusions.open")}
        </span>
      </summary>
      <div className="flex flex-col gap-4 border-t border-line p-4">
        {busy && !loaded ? (
          <p role="status" className={hintText}>
            {t("sync.exclusions.loading")}
          </p>
        ) : error !== "" ? (
          <Notice tone="danger">{error}</Notice>
        ) : view === null ? null : (
          <>
            <div>
              <p className="text-sm leading-6 text-ink-muted">
                {t("sync.exclusions.hint")}
              </p>
              {view.usingDefaults ? (
                <p className={`mt-1 ${hintText}`}>
                  {t("sync.exclusions.defaults")}
                </p>
              ) : null}
            </div>
            <input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              className={control}
              placeholder={t("sync.exclusions.search")}
            />
            <div className="max-h-64 overflow-auto rounded-md border border-hairline">
              {candidates.length === 0 ? (
                <p className="px-3 py-4 text-sm text-ink-muted">
                  {t("sync.exclusions.empty")}
                </p>
              ) : (
                candidates.map((candidate) => {
                  const ignored =
                    overrides[candidate.path] ?? candidate.ignored;
                  return (
                    <label
                      key={candidate.path}
                      className="flex min-h-10 items-center gap-3 border-t border-hairline px-3 py-2 first:border-t-0 hover:bg-select-fill"
                    >
                      <input
                        type="checkbox"
                        checked={!ignored}
                        onChange={() => {
                          const nextIgnored = !ignored;
                          setOverrides((current) => ({
                            ...current,
                            [candidate.path]: nextIgnored,
                          }));
                          setDocument((current) =>
                            appendRule(
                              current,
                              `${nextIgnored ? "" : "!"}${exactPattern(candidate.path)}`,
                            ),
                          );
                        }}
                      />
                      <span className="min-w-0 break-all font-mono text-xs text-ink">
                        {candidate.path}
                      </span>
                    </label>
                  );
                })
              )}
            </div>
            {sensitiveIgnored ? (
              <Notice tone="danger">
                {t("sync.exclusions.sensitiveWarning")}
              </Notice>
            ) : null}
            <details className="rounded-md border border-hairline bg-surface-subtle">
              <summary className="cursor-pointer px-3 py-2 text-sm font-medium text-ink">
                {t("sync.exclusions.advanced")}
              </summary>
              <div className="border-t border-hairline p-3">
                <textarea
                  value={document}
                  onChange={(event) => setDocument(event.target.value)}
                  rows={12}
                  spellCheck={false}
                  className={`${control} min-h-52 resize-y font-mono text-xs`}
                  aria-label={t("sync.exclusions.rules")}
                />
                <p className={`mt-2 ${hintText}`}>
                  {t("sync.exclusions.syntax")}
                </p>
              </div>
            </details>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                kind="primary"
                disabled={busy || document === view.document}
                onClick={() => {
                  setBusy(true);
                  setError("");
                  void api
                    .saveSyncExclusions(document)
                    .then((next) => {
                      setView(next);
                      setDocument(next.document);
                      setOverrides({});
                      onSaved?.(next);
                    })
                    .catch((caught) => {
                      setError(
                        failureCode(caught) === "sync_ignore_invalid"
                          ? t("sync.exclusions.invalid")
                          : t("sync.exclusions.saveFailed"),
                      );
                    })
                    .finally(() => setBusy(false));
                }}
              >
                {t("sync.exclusions.save")}
              </Button>
              <span className={hintText}>{t("sync.exclusions.shared")}</span>
            </div>
          </>
        )}
      </div>
    </details>
  );
}
