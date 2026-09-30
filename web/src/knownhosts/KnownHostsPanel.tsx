import { useEffect, useState } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import { failureCode } from "../api/client";
import { knownHostsApi, type KnownHostCandidate, type KnownHostEntry, type KnownHostsApi, type KnownHostsResponse } from "../api/knownHosts";
import {
  CheckboxField,
  Field,
  control,
  sectionHeading,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { Button, Card, Notice } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { useRequestGeneration } from "../ui/useRequestGeneration";

type KnownHostsPanelProps = { api?: KnownHostsApi };
type CandidateSort = "host" | "type" | "fingerprint" | "trust";
type TrustedSort = Exclude<CandidateSort, "trust">;

// sshc neither reads nor writes ~/.ssh/known_hosts through a symbolic link, so
// every operation on the file fails the same way. That message says why and that
// ssh, which follows the link, can still add a host's key. Any other failure keeps
// the message of the operation that failed.
function failureMessage(t: Translate, failure: unknown, otherwise: string): string {
  return failureCode(failure) === "known_hosts_symlink" ? t("kh.symlink") : otherwise;
}

export function KnownHostsPanel({ api = knownHostsApi }: KnownHostsPanelProps) {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const [listing, setListing] = useState<KnownHostsResponse | null>(null);
  const [pending, setPending] = useState<KnownHostEntry | null>(null);
  const [scanHost, setScanHost] = useState("");
  // スキャンの結果には、身元を証明しないという注意を必ず添える。文は engine の英語の
  // notice ではなく、画面の言語で出す。
  const [scanned, setScanned] = useState(false);
  const [candidates, setCandidates] = useState<KnownHostCandidate[]>([]);
  const [adding, setAdding] = useState<KnownHostCandidate | null>(null);
  const [expectedFingerprint, setExpectedFingerprint] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const candidateSort = useTableSort<CandidateSort>("host");
  const trustedSort = useTableSort<TrustedSort>("host");

  // The search runs on every keystroke, and answers can arrive out of order:
  // only the answer to the latest query may replace the listing.
  const listingRequests = useRequestGeneration();

  useEffect(() => {
    const isCurrent = listingRequests.begin();
    void api
      .knownHosts("")
      .then((result) => {
        if (isCurrent()) setListing(result);
      })
      .catch((failure: unknown) => {
        if (isCurrent()) setError(failureMessage(t, failure, t("kh.unreadable")));
      });
    return listingRequests.retire;
  }, [api, listingRequests, t]);

  async function search(next: string) {
    const isCurrent = listingRequests.begin();
    setError("");
    try {
      const result = await api.knownHosts(next);
      if (isCurrent()) setListing(result);
    } catch (failure) {
      if (isCurrent()) setError(failureMessage(t, failure, t("kh.unreadable")));
    }
  }

  // 削除の失敗は確認ダイアログの中に出し、ダイアログは開いたままにする。
  async function confirmDelete() {
    if (!pending || !listing) return;
    setDeleteError("");
    const removed = await api
      .deleteKnownHosts([{ line: pending.line, digest: pending.digest }], listing.path)
      .catch((failure: unknown) => {
        setDeleteError(failureMessage(t, failure, t("kh.removeFailed")));
        return null;
      });
    if (removed === null) {
      return;
    }
    setStatus(t("kh.removed", { id: removed.transactionId }));
    setPending(null);
    await search(query);
  }

  async function scan() {
    setError("");
    try {
      const result = await api.scanKnownHosts(scanHost, 22);
      setScanned(true);
      setCandidates(result.candidates);
    } catch {
      setError(t("kh.scanFailed"));
    }
  }

  function resetAdd() {
    setExpectedFingerprint("");
    setAcknowledged(false);
  }

  function openAdd(candidate: KnownHostCandidate) {
    resetAdd();
    setAdding(candidate);
  }

  function closeAdd() {
    resetAdd();
    setAdding(null);
  }

  async function confirmAdd() {
    if (!adding) return;
    const typed = expectedFingerprint.trim();
    if (typed !== "" && typed !== adding.fingerprint) {
      setError(t("kh.fingerprintMismatch", { typed, scanned: adding.fingerprint }));
      return;
    }
    setError("");
    try {
      const result = await api.addKnownHost(
        { host: adding.host, port: adding.port, keyType: adding.keyType, key: adding.key },
        typed,
        acknowledged,
      );
      setStatus(t("kh.added", { host: adding.host, id: result.transactionId }));
      closeAdd();
      await search(query);
    } catch (failure) {
      const code = failureCode(failure);
      setError(
        failureMessage(
          t,
          failure,
          code === "" ? t("kh.addFailed") : t("kh.addFailedCode", { code }),
        ),
      );
      closeAdd();
    }
  }

  const provenOrAcknowledged = expectedFingerprint.trim() !== "" || acknowledged;
  // スキャンで見つけた鍵は、どれもまだ確かめていない。
  const candidateTrustText = t("kh.unverified");
  const candidateSortValue = (candidate: KnownHostCandidate, column: CandidateSort): SortValue => {
    switch (column) {
      case "host": return candidate.host;
      case "type": return candidate.keyType;
      case "fingerprint": return candidate.fingerprint;
      case "trust": return candidateTrustText;
    }
  };
  const trustedHostText = (entry: KnownHostEntry) => (entry.hashed ? t("kh.hashed") : entry.hosts.join(", "));
  const trustedSortValue = (entry: KnownHostEntry, column: TrustedSort): SortValue => {
    switch (column) {
      case "host": return trustedHostText(entry);
      case "type": return entry.keyType;
      case "fingerprint": return entry.fingerprint;
    }
  };
  const displayedCandidates = candidateSort.sorted(candidates, candidateSortValue);
  const displayedEntries = trustedSort.sorted(listing?.entries ?? [], trustedSortValue);

  return (
    <section aria-label={t("kh.heading")} className="mx-auto flex w-full max-w-5xl flex-col gap-6 [&_button]:min-h-10 sm:[&_button]:min-h-0">
      <PageHeader title={t("kh.heading")} description={t("kh.pageDescription")} />
      <MetricGrid className="sm:grid-cols-3 lg:grid-cols-3">
        {([
          [t("kh.metricEntries"), listing?.entries.length ?? 0],
          [t("kh.metricHashed"), listing?.entries.filter((entry) => entry.hashed).length ?? 0],
          [t("kh.metricCandidates"), candidates.length],
        ] as const).map(([label, value], index) => (
          <MetricCard key={String(label)} label={String(label)} value={value} compact attention={index === 2 && candidates.length > 0} />
        ))}
      </MetricGrid>

      <p aria-live="polite" className="text-sm text-ink-muted">
        {status}
      </p>
      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}


      <Card as="section" radius="md" aria-labelledby="known-hosts-scan-heading">
        <div className="flex flex-wrap items-end justify-between gap-4 bg-surface-subtle px-4 py-4">
          <div>
            <h3 id="known-hosts-scan-heading" className={sectionHeading}>
              {t("kh.scanHeading")}
            </h3>
            <p className="mt-1 text-xs text-ink-muted">{t("kh.pageDescription")}</p>
          </div>
          <div className="flex min-w-64 flex-1 flex-wrap items-end gap-2 sm:max-w-xl">
            <div className="min-w-48 flex-1">
              <Field label={t("kh.hostToScan")}>
                <input
                  value={scanHost}
                  onChange={(event) => setScanHost(event.target.value)}
                  className={control}
                />
              </Field>
            </div>
            <Button kind="primary" onClick={() => void scan()}>
              {t("kh.scan")}
            </Button>
          </div>
        </div>

        {scanned ? <p className="border-t border-notice-line bg-notice px-4 py-3 text-sm text-notice-ink">{t("kh.unverifiedNotice")}</p> : null}
        {candidates.length > 0 ? (
          <div className="overflow-x-auto px-4 py-3">
            <table className="w-full text-sm">
              <caption className="mb-2 text-left text-ink-muted">{t("kh.scanCandidates")}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <SortableTableHeader column="host" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnHost")}</SortableTableHeader>
                  <SortableTableHeader column="type" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnType")}</SortableTableHeader>
                  <SortableTableHeader column="fingerprint" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnFingerprint")}</SortableTableHeader>
                  <SortableTableHeader column="trust" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnTrust")}</SortableTableHeader>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnActions")}</th>
                </tr>
              </thead>
              <tbody>
                {displayedCandidates.map((candidate) => (
                  <tr key={`${candidate.host}-${candidate.fingerprint}`} className="border-b border-line last:border-b-0">
                    <td className="py-2 pr-3">{candidate.host}</td>
                    <td className="py-2 pr-3 text-ink-muted">{candidate.keyType}</td>
                    <td className="py-2 pr-3 font-mono text-xs text-ink-muted">{candidate.fingerprint}</td>

                    <td className="py-2 pr-3"><span className="rounded-full bg-notice px-2 py-1 text-xs font-medium text-notice-ink">{candidateTrustText}</span></td>
                    <td className="py-2">
                      <Button
                        onClick={() => openAdd(candidate)}
                      >
                        {t("kh.add")}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}

        {adding ? (
          <div className="border-t border-notice-line bg-notice p-4 text-sm">
            <h3 className="font-medium text-notice-ink">{t("kh.addHeading")}</h3>
            <p className="text-ink-muted">{t("kh.addExplain", { host: adding.host })}</p>
            <p className="text-ink-muted">
              {adding.keyType} · {adding.fingerprint}
            </p>
            <Field label={t("kh.expectedFingerprint")}>
              <input
                value={expectedFingerprint}
                onChange={(event) => setExpectedFingerprint(event.target.value)}
                className={control}
              />
            </Field>
            <CheckboxField
              label={t("kh.acknowledge")}
              checked={acknowledged}
              onChange={setAcknowledged}
            />
            <div className="mt-2 flex gap-2">
              <button
                type="button"
                disabled={!provenOrAcknowledged}
                onClick={() => void confirmAdd()}
                className="rounded-md bg-accent px-3 py-1.5 font-medium text-accent-ink disabled:bg-line disabled:text-ink-faint"
              >
                {t("kh.addToKnownHosts")}
              </button>
              <Button onClick={closeAdd}>
                {t("kh.cancel")}
              </Button>
            </div>
          </div>
        ) : null}
      </Card>

      <Card as="section" radius="md" aria-labelledby="known-hosts-trusted-heading">
        <div className="flex flex-wrap items-end justify-between gap-4 border-b border-line px-4 py-4">
          <div>
            <h3 id="known-hosts-trusted-heading" className={sectionHeading}>
              {t("kh.trustedHeading")}
            </h3>
            {listing ? <p className="mt-1 font-mono text-xs text-ink-faint">{listing.path}</p> : null}
          </div>
          <div className="w-full sm:w-72">
            <Field label={t("kh.search")}>
              <input
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  void search(event.target.value);
                }}
                className={control}
              />
            </Field>
          </div>
        </div>

        {listing ? (
          <div className="px-4 pb-4">
            <table className="block w-full text-sm sm:table">
              <caption className="sr-only">{listing.path}</caption>
              <thead className="hidden sm:table-header-group">
                <tr className={tableHeadRow}>
                  <SortableTableHeader column="host" {...trustedSort.headerProps} className={tableHeadCell}>{t("kh.columnHost")}</SortableTableHeader>
                  <SortableTableHeader column="type" {...trustedSort.headerProps} className={tableHeadCell}>{t("kh.columnType")}</SortableTableHeader>
                  <SortableTableHeader column="fingerprint" {...trustedSort.headerProps} className={tableHeadCell}>{t("kh.columnFingerprint")}</SortableTableHeader>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnActions")}</th>
                </tr>
              </thead>
              <tbody className="block sm:table-row-group">
                {displayedEntries.map((item) => (
                  <tr key={`${item.line}-${item.digest}`} className="grid gap-2 border-b border-line py-3 last:border-b-0 sm:table-row sm:py-0">
                    <td className="flex min-w-0 items-start justify-between gap-4 sm:table-cell sm:py-2 sm:pr-3">
                      <span aria-hidden="true" className="shrink-0 text-xs font-medium uppercase tracking-wide text-ink-muted sm:hidden">{t("kh.columnHost")}</span>
                      <span className="min-w-0 break-all text-right sm:text-left">{trustedHostText(item)}</span>
                    </td>
                    <td className="flex min-w-0 items-start justify-between gap-4 text-ink-muted sm:table-cell sm:py-2 sm:pr-3">
                      <span aria-hidden="true" className="shrink-0 text-xs font-medium uppercase tracking-wide sm:hidden">{t("kh.columnType")}</span>
                      <span className="min-w-0 break-all text-right sm:text-left">{item.keyType}</span>
                    </td>
                    <td className="flex min-w-0 items-start justify-between gap-4 font-mono text-xs text-ink-muted sm:table-cell sm:py-3 sm:pr-3">
                      <span aria-hidden="true" className="shrink-0 font-sans font-medium uppercase tracking-wide sm:hidden">{t("kh.columnFingerprint")}</span>
                      <span className="min-w-0 break-all text-right sm:text-left">{item.fingerprint}</span>
                    </td>
                    <td className="flex justify-end sm:table-cell sm:py-2">
                      <Button
                        className="w-full sm:w-auto"
                        onClick={() => {
                          setDeleteError("");
                          setPending(item);
                        }}
                      >
                        {t("kh.delete")}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}

      </Card>
      {pending === null ? null : (
        <ConfirmDialog
          id="known-host-delete-heading"
          heading={t("kh.delete")}
          body={<p className="text-sm text-ink-muted">{t("kh.confirmRemove", { line: pending.line, fingerprint: pending.fingerprint })}</p>}
          confirmLabel={t("kh.confirmDelete")}
          cancelLabel={t("kh.cancel")}
          onConfirm={confirmDelete}
          onCancel={() => setPending(null)}
          error={deleteError}
        />
      )}
    </section>
  );
}
