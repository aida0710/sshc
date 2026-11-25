import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { failureCode } from "../api/client";
import type { SyncApi, SyncExclusions } from "../api/sync";
import { CodeField } from "../editor/CodeField";
import { useLanguage } from "../i18n/context";
import { DraftSaveBar } from "../ui/DraftSaveBar";
import { control, hintText } from "../ui/form";
import { Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { useToast } from "../ui/useToast";
import { PanelState } from "../ui/PanelState";

type Props = {
  api: Pick<SyncApi, "syncExclusions" | "saveSyncExclusions">;
  initial?: SyncExclusions;
  onSaved?: (value: SyncExclusions) => void;
  onDirtyChange?: (dirty: boolean) => void;
  // onDiscardReady receives the function that gives up the changes.
  onDiscardReady?: (discard: (() => void) | null) => void;
};

function exactPattern(path: string): string {
  const escaped = path.replace(/([\\*?\[\]#!])/g, "\\$1");
  return `/${escaped}`;
}

function appendRule(document: string, rule: string): string {
  if (document === "") return `${rule}\n`;
  return `${document}${document.endsWith("\n") ? "" : "\n"}${rule}\n`;
}

export function SyncExclusionsPanel({ api, initial, onSaved, onDirtyChange, onDiscardReady }: Props) {
  const { t } = useLanguage();
  const showToast = useToast();
  const [view, setView] = useState<SyncExclusions | null>(initial ?? null);
  const [document, setDocument] = useState(initial?.document ?? "");
  const [loaded, setLoaded] = useState(initial !== undefined);
  const [search, setSearch] = useState("");
  const [overrides, setOverrides] = useState<Record<string, boolean>>({});
  const operation = useAsyncOperation();

  async function load() {
    if (loaded || operation.busy) return;
    await operation.run(() => api.syncExclusions(), {
      apply: (next) => {
        setView(next);
        setDocument(next.document);
        setLoaded(true);
      },
      describe: () => t("sync.exclusions.loadFailed"),
    });
  }

  const changed = view !== null && document !== view.document;
  // What was edited here is given up when the page goes away, so a panel that goes away has no changes.
  useLayoutEffect(() => {
    onDirtyChange?.(changed);
    return () => onDirtyChange?.(false);
  }, [changed, onDirtyChange]);

  function discard() {
    if (view === null) return;
    setDocument(view.document);
    setOverrides({});
    operation.clearError();
  }
  // The page asks to give up the changes when it goes on to another one.
  const latestDiscard = useRef(discard);
  latestDiscard.current = discard;
  useEffect(() => {
    onDiscardReady?.(() => latestDiscard.current());
    return () => onDiscardReady?.(null);
  }, [onDiscardReady]);

  async function save() {
    await operation.run(() => api.saveSyncExclusions(document), {
      apply: (next) => {
        setView(next);
        setDocument(next.document);
        setOverrides({});
        showToast(t("draft.saved"), { tone: "success" });
        onSaved?.(next);
      },
      describe: (caught) =>
        failureCode(caught) === "sync_ignore_invalid"
          ? t("sync.exclusions.invalid")
          : t("sync.exclusions.saveFailed"),
    });
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
        candidate.path.startsWith("connections/") ||
        candidate.path.startsWith("keys/"))
    );
  });

  return (
    <details
      className="border-t border-line pt-4"
      onToggle={(event) => {
        if (event.currentTarget.open) void load();
      }}
    >
      <DisclosureSummary>
        <span className="min-w-0 grow text-sm font-semibold text-ink">{t("sync.exclusions.heading")}</span>
        <span className={hintText}>
          {loaded
            ? t("sync.exclusions.summary", {
                included: (view?.candidates.length ?? 0) - ignoredCount,
                ignored: ignoredCount,
              })
            : t("sync.exclusions.open")}
        </span>
      </DisclosureSummary>
      <div className="flex flex-col gap-4 border-t border-line p-4">
        {operation.busy && !loaded ? (
          <PanelState layout="line" tone="loading" title={t("sync.exclusions.loading")} />
        ) : view === null ? (
          operation.error === "" ? null : <Notice tone="danger">{operation.error}</Notice>
        ) : (
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
              aria-label={t("sync.exclusions.search")}
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
                      className="flex min-h-10 items-center gap-3 border-t border-hairline px-3 py-2 first:border-t-0 hover:bg-hover"
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
                      <span className="min-w-0 [overflow-wrap:anywhere] font-mono text-xs text-ink">
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
              <DisclosureSummary className="px-3 py-2 text-sm font-medium text-ink">
                {t("sync.exclusions.advanced")}
              </DisclosureSummary>
              <div className="border-t border-hairline p-3">
                <CodeField
                  label={t("sync.exclusions.rules")}
                  value={document}
                  onChange={setDocument}
                  minLines={12}
                  maxLines={30}
                />
                <p className={`mt-2 ${hintText}`}>
                  {t("sync.exclusions.syntax")}
                </p>
              </div>
            </details>
            <p className={hintText}>{t("sync.exclusions.shared")}</p>
            {changed ? (
              <DraftSaveBar saving={operation.busy} failure={operation.error} onDiscard={discard} onSave={() => void save()} />
            ) : null}
          </>
        )}
      </div>
    </details>
  );
}
