import { useEffect, useState } from "react";
import { aboutApi, type About, type AboutApi } from "../api/about";
import { updateApi, type UpdateApi } from "../api/update";
import { useLanguage } from "../i18n/context";
import { UpdateControls } from "../shell/UpdateControls";
import { explainedUpdateReason, updateMessage } from "../shell/updateMessage";
import { useUpdateStatus } from "../shell/useUpdateStatus";
import { formatDateTime } from "../ui/format";
import { Button, Notice } from "../ui/surface";
import { ReleaseNotes } from "./ReleaseNotes";

// Seven characters keep the revision readable while the link uses the complete SHA.
const shortRevisionLength = 7;

type AboutPageProps = { api?: AboutApi; updates?: UpdateApi };

export function AboutPage({ api = aboutApi, updates = updateApi }: AboutPageProps) {
  const { t, locale } = useLanguage();
  const [about, setAbout] = useState<About | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [checking, setChecking] = useState(false);
  const [checkFailed, setCheckFailed] = useState(false);
  const { status, setStatus } = useUpdateStatus(updates);

  useEffect(() => {
    let active = true;
    void api.about().then((loaded) => {
      if (active) setAbout(loaded);
    }).catch(() => { if (active) setLoadFailed(true); });
    return () => { active = false; };
  }, [api]);

  async function checkUpdate() {
    if (checking) return;
    setChecking(true);
    setCheckFailed(false);
    try {
      setStatus(await updates.checkUpdate());
    } catch {
      setCheckFailed(true);
    } finally {
      setChecking(false);
    }
  }

  const unknown = t("about.unknown");
  const reason = status === null ? undefined : explainedUpdateReason(status);
  const revision = about?.revision ?? "";
  const commitURL = /^[a-f0-9]{40,64}$/i.test(revision) ? `https://github.com/aida0710/sshc/commit/${revision}` : undefined;
  const buildFields = about === null ? [] : [
    [t("about.version"), about.version || unknown],
    [t("about.commit"), commitURL === undefined ? unknown : <a href={commitURL} target="_blank" rel="noopener noreferrer" className="text-accent underline">{revision.slice(0, shortRevisionLength)}</a>],
    [t("about.commitTime"), about.commitTime === undefined ? unknown : formatDateTime(about.commitTime, locale)],
    [t("about.modified"), about.modified === undefined ? unknown : t(about.modified ? "about.modifiedYes" : "about.modifiedNo")],
    [t("about.goVersion"), about.goVersion || unknown],
    [t("about.platform"), `${about.os || unknown} / ${about.arch || unknown}`],
    [t("about.manager"), about.manager === "unknown" ? t("about.unmanaged") : about.manager === "homebrew" ? "Homebrew" : about.manager],
    [t("about.executable"), about.executable || unknown],
  ] as const;

  return (
    <section aria-labelledby="about-heading" className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="border-b border-line pb-3">
        <h2 id="about-heading" className="text-xl font-semibold text-ink">{t("section.about")}</h2>
      </header>
      <section aria-labelledby="about-running-heading" className="rounded border border-line bg-card p-4">
        <h3 id="about-running-heading" className="mb-3 font-semibold text-ink">{t("about.running")}</h3>
        {loadFailed ? <Notice tone="danger">{t("about.loadFailed")}</Notice> : about === null ? <p role="status">{t("about.loading")}</p> : (
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-[auto_1fr]">
            {buildFields.map(([label, value]) => (
              <div key={label} className="contents">
                <dt className="text-ink-muted">{label}</dt>
                <dd className="min-w-0 break-all text-ink">{value}</dd>
              </div>
            ))}
          </dl>
        )}
      </section>
      <section aria-labelledby="about-update-heading" className="rounded border border-line bg-card p-4">
        <h3 id="about-update-heading" className="mb-3 font-semibold text-ink">{t("about.updates")}</h3>
        <div className="grid gap-3 text-sm text-ink-muted">
          <p role="status">{status?.latest === undefined ? t("about.noRelease") : status.available ? t("about.available", { version: status.latest }) : t("about.upToDate")}</p>
          <dl className="grid gap-x-6 gap-y-2 sm:grid-cols-[auto_1fr]">
            <dt>{t("about.latest")}</dt><dd className="text-ink">{status?.latest ?? unknown}</dd>
            <dt>{t("about.publishedAt")}</dt><dd>{status?.publishedAt === undefined ? unknown : formatDateTime(status.publishedAt, locale)}</dd>
            <dt>{t("about.checkedAt")}</dt><dd>{status?.checkedAt === undefined ? t("about.notChecked") : formatDateTime(status.checkedAt, locale)}</dd>
          </dl>
          {checkFailed || status?.checkError ? <Notice tone="danger">{t("update.checkFailed")}{status?.checkError ? <span className="mt-1 block break-words">{status.checkError}</span> : null}</Notice> : null}
          {reason === undefined ? null : <p>{t(updateMessage(reason))}</p>}
          <div className="flex flex-wrap items-baseline gap-4">
            <Button disabled={checking} onClick={() => { void checkUpdate(); }}>{t(checking ? "about.checking" : "about.check")}</Button>
            {status === null ? null : <UpdateControls status={status} api={updates} onStart={(job) => setStatus({ ...status, canUpdate: false, job })} />}
          </div>
        </div>
      </section>
      <ReleaseNotes releases={status?.releases ?? []} current={status?.current ?? about?.version ?? ""} latest={status?.latest} />
    </section>
  );
}
