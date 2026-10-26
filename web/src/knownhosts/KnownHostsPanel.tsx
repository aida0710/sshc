import { useEffect, useState } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import { failureCode } from "../api/client";
import { knownHostsApi, type KnownHostCandidate, type KnownHostEntry, type KnownHostsApi, type KnownHostsResponse } from "../api/knownHosts";
import {
  CheckboxField,
  Field,
  control,
  tableCell,
  tableCellOnWide,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { revealOnHover } from "../ui/ActionMenu";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { PageHeader, PageSection, SummaryStat, SummaryStats } from "../ui/page";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { PanelState } from "../ui/PanelState";
import { usePendingAction } from "../ui/usePendingAction";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { useToast } from "../ui/useToast";

type KnownHostsPanelProps = { api?: KnownHostsApi };
type CandidateSort = "host" | "type" | "fingerprint" | "trust";
type TrustedSort = Exclude<CandidateSort, "trust">;
// KnownHostsAction は、相手のホストか known_hosts の書き込みを待つ操作である。押したボタンに
// 回転の印を出し、待つ間はもう一方も始めない。
type KnownHostsAction = "scan" | "addToKnownHosts";

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
  const action = usePendingAction<KnownHostsAction>();
  const [adding, setAdding] = useState<KnownHostCandidate | null>(null);
  const [expectedFingerprint, setExpectedFingerprint] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  // 追加と削除の結果は、確かめる欄やダイアログを閉じたあとに出すので、トーストで知らせる。
  const showToast = useToast();
  const [error, setError] = useState("");
  const [deleteError, setDeleteError] = useState("");
  // firstListingAnswered は、開いたときの一覧の読み込みが答えた（読めた、または読めなかった）
  // ことを表す。error はスキャンや検索を始めるたびに消えるので、読み込み中かどうかには使えない。
  const [firstListingAnswered, setFirstListingAnswered] = useState(false);
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
      })
      .finally(() => {
        if (isCurrent()) setFirstListingAnswered(true);
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
    showToast(t("kh.removed", { id: removed.transactionId }), { tone: "success" });
    setPending(null);
    await search(query);
  }

  async function scan() {
    await action.start("scan", async () => {
      setError("");
      try {
        const result = await api.scanKnownHosts(scanHost, 22);
        setScanned(true);
        setCandidates(result.candidates);
      } catch {
        setError(t("kh.scanFailed"));
      }
    });
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
    const candidate = adding;
    const typed = expectedFingerprint.trim();
    if (typed !== "" && typed !== candidate.fingerprint) {
      setError(t("kh.fingerprintMismatch", { typed, scanned: candidate.fingerprint }));
      return;
    }
    await action.start("addToKnownHosts", async () => {
      setError("");
      try {
        const result = await api.addKnownHost(
          { host: candidate.host, port: candidate.port, keyType: candidate.keyType, key: candidate.key },
          typed,
          acknowledged,
        );
        showToast(t("kh.added", { host: candidate.host, id: result.transactionId }), { tone: "success" });
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
    });
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

  // 最初の一覧が届くまでは、件数を「0」と言わない。
  if (!firstListingAnswered) {
    return <PanelState tone="loading" title={t("kh.loading")} />;
  }

  return (
    <section aria-label={t("kh.heading")} className="flex flex-col gap-[22px]">
      <PageHeader title={t("kh.heading")} />
      {/* 読めなかった known_hosts の件数は分からないので、件数の欄ごと出さない。 */}
      {listing === null ? null : (
        <SummaryStats>
          {([
            [t("kh.metricEntries"), listing.entries.length],
            [t("kh.metricHashed"), listing.entries.filter((entry) => entry.hashed).length],
            [t("kh.metricCandidates"), candidates.length],
          ] as const).map(([label, value], index) => (
            <SummaryStat key={String(label)} label={String(label)} value={value} attention={index === 2 && candidates.length > 0} />
          ))}
        </SummaryStats>
      )}

      {error ? <Notice tone="danger">{error}</Notice> : null}

      <PageSection heading={t("kh.scanHeading")} headingId="known-hosts-scan-heading" hint={t("kh.scanHint")}>
        <div className="flex flex-wrap items-center gap-2">
          <input
            aria-label={t("kh.hostToScan")}
            placeholder={t("kh.hostToScan")}
            value={scanHost}
            onChange={(event) => setScanHost(event.target.value)}
            className={`${control} sm:max-w-[360px]`}
          />
          <Button kind="primary" busy={action.running === "scan"} disabled={action.running !== null} onClick={() => void scan()}>
            {t("kh.scan")}
          </Button>
        </div>

        {scanned ? <p className="mt-3 rounded-[9px] bg-notice px-3 py-2.5 text-[12.5px] leading-[19px] text-notice-ink">{t("kh.unverifiedNotice")}</p> : null}
        {candidates.length > 0 ? (
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-[13px]">
              <caption className="mb-2 text-left text-xs text-ink-faint">{t("kh.scanCandidates")}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <SortableTableHeader column="host" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnHost")}</SortableTableHeader>
                  <SortableTableHeader column="type" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnType")}</SortableTableHeader>
                  <SortableTableHeader column="fingerprint" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnFingerprint")}</SortableTableHeader>
                  <SortableTableHeader column="trust" {...candidateSort.headerProps} className={tableHeadCell}>{t("kh.columnTrust")}</SortableTableHeader>
                  <th scope="col" className={tableHeadCell}><span className="sr-only">{t("kh.columnActions")}</span></th>
                </tr>
              </thead>
              <tbody>
                {displayedCandidates.map((candidate) => (
                  <tr key={`${candidate.host}-${candidate.fingerprint}`} className="border-b border-line last:border-b-0">
                    <td className={tableCell}>{candidate.host}</td>
                    <td className={`${tableCell} font-mono text-xs text-ink-muted`}>{candidate.keyType}</td>
                    <td className={`${tableCell} font-mono text-xs text-ink-muted`}>{candidate.fingerprint}</td>
                    <td className={tableCell}><span className="rounded-full bg-notice px-2 py-0.5 text-[11.5px] text-notice-ink">{candidateTrustText}</span></td>
                    <td className={`${tableCell} text-right`}>
                      <Button onClick={() => openAdd(candidate)}>{t("kh.add")}</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}

        {adding ? (
          <div className="mt-3 flex flex-col gap-3 rounded-[10px] bg-notice p-4 text-sm">
            <div>
              <h4 className="font-medium text-notice-ink">{t("kh.addHeading")}</h4>
              <p className="text-ink-muted">{t("kh.addExplain", { host: adding.host })}</p>
              <p className="font-mono text-xs text-ink-muted">
                {adding.keyType} · {adding.fingerprint}
              </p>
            </div>
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
            <div className="flex gap-2">
              <Button
                kind="primary"
                busy={action.running === "addToKnownHosts"}
                disabled={!provenOrAcknowledged || action.running !== null}
                onClick={() => void confirmAdd()}
              >
                {t("kh.addToKnownHosts")}
              </Button>
              <Button disabled={action.running === "addToKnownHosts"} onClick={closeAdd}>
                {t("kh.cancel")}
              </Button>
            </div>
          </div>
        ) : null}
      </PageSection>

      <PageSection
        heading={t("kh.trustedHeading")}
        headingId="known-hosts-trusted-heading"
        meta={listing?.path}
        actions={(
          <span className="relative block w-full sm:w-60">
            <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
            <input
              aria-label={t("kh.search")}
              placeholder={t("kh.search")}
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                void search(event.target.value);
              }}
              className={`${control} pl-8`}
            />
          </span>
        )}
      >
        {listing ? (
          <table className="block w-full text-[13px] sm:table">
            <caption className="sr-only">{listing.path}</caption>
            <thead className="hidden sm:table-header-group">
              <tr className={tableHeadRow}>
                <SortableTableHeader column="host" {...trustedSort.headerProps} className={tableHeadCell}>{t("kh.columnHost")}</SortableTableHeader>
                <SortableTableHeader column="type" {...trustedSort.headerProps} className={tableHeadCell}>{t("kh.columnType")}</SortableTableHeader>
                <SortableTableHeader column="fingerprint" {...trustedSort.headerProps} className={tableHeadCell}>{t("kh.columnFingerprint")}</SortableTableHeader>
                <th scope="col" className={tableHeadCell}><span className="sr-only">{t("kh.columnActions")}</span></th>
              </tr>
            </thead>
            <tbody className="block sm:table-row-group">
              {displayedEntries.map((item) => (
                <tr key={`${item.line}-${item.digest}`} className="group/row grid gap-2 border-b border-line py-3 last:border-b-0 hover:bg-hover sm:table-row sm:py-0">
                  <td className={`flex min-w-0 items-start justify-between gap-4 ${tableCellOnWide}`}>
                    <span aria-hidden="true" className="shrink-0 text-xs text-ink-faint sm:hidden">{t("kh.columnHost")}</span>
                    <span className="min-w-0 break-all text-right sm:text-left">{trustedHostText(item)}</span>
                  </td>
                  <td className={`flex min-w-0 items-start justify-between gap-4 font-mono text-xs text-ink-muted ${tableCellOnWide}`}>
                    <span aria-hidden="true" className="shrink-0 font-sans text-ink-faint sm:hidden">{t("kh.columnType")}</span>
                    <span className="min-w-0 break-all text-right sm:text-left">{item.keyType}</span>
                  </td>
                  <td className={`flex min-w-0 items-start justify-between gap-4 font-mono text-xs text-ink-muted ${tableCellOnWide}`}>
                    <span aria-hidden="true" className="shrink-0 font-sans text-ink-faint sm:hidden">{t("kh.columnFingerprint")}</span>
                    <span className="min-w-0 break-all text-right sm:text-left">{item.fingerprint}</span>
                  </td>
                  <td className={`flex justify-end ${tableCellOnWide} sm:text-right`}>
                    {/* Removing is offered on the row under the pointer (always on touch), in red words only. */}
                    <Button
                      kind="dangerText"
                      className={revealOnHover}
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
        ) : null}
      </PageSection>
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
