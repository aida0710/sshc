import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { failureCode } from "../api/client";
import {
  integrationsApi,
  type IntegrationsApi,
  type KnownHostCandidate,
  type KnownHostEntry,
  type KnownHostsResponse,
} from "../api/integrations";
import {
  CheckboxField,
  Field,
  control,
  sectionHeading,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";

type KnownHostsPanelProps = { api?: IntegrationsApi };

export function KnownHostsPanel({ api = integrationsApi }: KnownHostsPanelProps) {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const [listing, setListing] = useState<KnownHostsResponse | null>(null);
  const [pending, setPending] = useState<KnownHostEntry | null>(null);
  const [scanHost, setScanHost] = useState("");
  const [notice, setNotice] = useState("");
  const [candidates, setCandidates] = useState<KnownHostCandidate[]>([]);
  const [adding, setAdding] = useState<KnownHostCandidate | null>(null);
  const [expectedFingerprint, setExpectedFingerprint] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    void api
      .knownHosts("")
      .then((result) => {
        if (active) setListing(result);
      })
      .catch(() => {
        if (active) setError(t("kh.unreadable"));
      });
    return () => {
      active = false;
    };
  }, [api, t]);

  async function search(next: string) {
    setError("");
    try {
      setListing(await api.knownHosts(next));
    } catch {
      setError(t("kh.unreadable"));
    }
  }

  async function confirmDelete() {
    if (!pending || !listing) return;
    setError("");
    try {
      const result = await api.deleteKnownHosts(
        [{ line: pending.line, digest: pending.digest }],
        listing.path,
      );
      setStatus(t("kh.removed", { id: result.transactionId }));
      setPending(null);
      await search(query);
    } catch {
      setError(t("kh.removeFailed"));
      setPending(null);
    }
  }

  async function scan() {
    setError("");
    try {
      const result = await api.scanKnownHosts(scanHost, 22);
      setNotice(result.notice);
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
        code === ""
          ? t("kh.addFailed")
          : t("kh.addFailedCode", { code }),
      );
      closeAdd();
    }
  }

  const provenOrAcknowledged = expectedFingerprint.trim() !== "" || acknowledged;

  return (
    <section aria-label={t("kh.heading")} className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("kh.heading")} description={t("kh.pageDescription")} />
      <div className="sshc-card flex flex-wrap divide-x divide-line overflow-hidden rounded-xl bg-card">
        {[
          [t("kh.metricEntries"), listing?.entries.length ?? 0],
          [t("kh.metricHashed"), listing?.entries.filter((entry) => entry.hashed).length ?? 0],
          [t("kh.metricCandidates"), candidates.length],
        ].map(([label, value], index) => (
          <div key={String(label)} className="flex min-w-40 flex-1 items-center justify-between gap-4 px-4 py-3">
            <span className="text-xs font-medium text-ink-muted">{label}</span>
            <span className={`font-mono text-lg font-semibold ${index === 2 && candidates.length > 0 ? "text-notice-ink" : "text-ink"}`}>{value}</span>
          </div>
        ))}
      </div>

      <p aria-live="polite" className="text-sm text-ink-muted">
        {status}
      </p>
      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}


      <section className="sshc-card overflow-hidden rounded-xl bg-card" aria-labelledby="known-hosts-scan-heading">
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

        {notice ? <p className="border-t border-notice-line bg-notice px-4 py-3 text-sm text-notice-ink">{notice}</p> : null}
        {candidates.length > 0 ? (
          <div className="overflow-x-auto px-4 py-3">
            <table className="w-full text-sm">
              <caption className="mb-2 text-left text-ink-muted">{t("kh.scanCandidates")}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnHost")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnType")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnFingerprint")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnTrust")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnActions")}</th>
                </tr>
              </thead>
              <tbody>
                {candidates.map((candidate) => (
                  <tr key={`${candidate.host}-${candidate.fingerprint}`} className="border-b border-line last:border-b-0">
                    <td className="py-2 pr-3">{candidate.host}</td>
                    <td className="py-2 pr-3 text-ink-muted">{candidate.keyType}</td>
                    <td className="py-2 pr-3 font-mono text-xs text-ink-muted">{candidate.fingerprint}</td>

                    <td className="py-2 pr-3"><span className="rounded-full bg-notice px-2 py-1 text-xs font-medium text-notice-ink">{t("kh.unverified")}</span></td>
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
      </section>

      <section className="sshc-card overflow-hidden rounded-xl bg-card" aria-labelledby="known-hosts-trusted-heading">
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
          <div className="overflow-x-auto px-4 pb-4">
            <table className="w-full text-sm">
              <caption className="sr-only">{listing.path}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnHost")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnType")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnFingerprint")}</th>
                  <th scope="col" className={tableHeadCell}>{t("kh.columnActions")}</th>
                </tr>
              </thead>
              <tbody>
                {listing.entries.map((item) => (
                  <tr key={`${item.line}-${item.digest}`} className="border-b border-line last:border-b-0">
                    <td className="py-2 pr-3">{item.hashed ? t("kh.hashed") : item.hosts.join(", ")}</td>
                    <td className="py-2 pr-3 text-ink-muted">{item.keyType}</td>
                    <td className="py-3 pr-3 font-mono text-xs text-ink-muted">{item.fingerprint}</td>
                    <td className="py-2">
                      <Button
                        onClick={() => setPending(item)}
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

        {pending ? (
          <div className="border-t border-line bg-surface-subtle p-4 text-sm">
            <p>
              {t("kh.confirmRemove", { line: pending.line, fingerprint: pending.fingerprint })}
            </p>
            <div className="mt-2 flex gap-2">
              <Button
                onClick={() => void confirmDelete()}
              >
                {t("kh.confirmDelete")}
              </Button>
              <Button
                onClick={() => setPending(null)}
              >
                {t("kh.cancel")}
              </Button>
            </div>
          </div>
        ) : null}
      </section>
    </section>
  );
}
