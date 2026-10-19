import { useEffect, useState } from "react";
import { aboutApi, type About, type AboutApi } from "../api/about";
import { updateApi, type UpdateApi } from "../api/update";
import { useLanguage } from "../i18n/context";
import { UpdateControls } from "../shell/UpdateControls";
import { explainedUpdateReason, updateMessage } from "../shell/updateMessage";
import { useUpdateStatus } from "../shell/useUpdateStatus";
import { formatDateTime } from "../ui/format";
import { Button, Notice } from "../ui/surface";
import { BuildDetails } from "./BuildDetails";
import { ReleaseNotes } from "./ReleaseNotes";
import { checkRequestFailedActivity, checkResultActivity, checkStartedActivity } from "./updateActivity";
import { UpdateActivityLog } from "./UpdateActivityLog";
import { useUpdateActivity } from "./useUpdateActivity";

type AboutPageProps = { api?: AboutApi; updates?: UpdateApi };

export function AboutPage({ api = aboutApi, updates = updateApi }: AboutPageProps) {
  const { t, locale } = useLanguage();
  const [about, setAbout] = useState<About | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [checking, setChecking] = useState(false);
  const { status, setStatus } = useUpdateStatus(updates);
  const activity = useUpdateActivity(status);

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
    activity.record(checkStartedActivity);
    try {
      const checked = await updates.checkUpdate();
      setStatus(checked);
      activity.record(checkResultActivity(checked));
    } catch {
      activity.record(checkRequestFailedActivity);
    } finally {
      setChecking(false);
    }
  }

  const reason = status === null ? undefined : explainedUpdateReason(status);
  const currentState = status?.latest === undefined
    ? t("about.noRelease")
    : status.available ? t("about.available", { version: status.latest }) : t("about.upToDate", { version: status.current });

  return (
    <section aria-labelledby="about-heading" className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="border-b border-line pb-3">
        <h2 id="about-heading" className="text-xl font-semibold text-ink">{t("section.about")}</h2>
      </header>
      <div className="rounded border border-line bg-card p-4">
        {loadFailed ? <Notice tone="danger">{t("about.loadFailed")}</Notice> : about === null ? <p role="status">{t("about.loading")}</p> : <BuildDetails about={about} />}
      </div>
      <section aria-labelledby="about-update-heading" className="rounded border border-line bg-card p-4">
        <h3 id="about-update-heading" className="mb-3 font-semibold text-ink">{t("about.updates")}</h3>
        <div className="grid gap-3 text-sm text-ink-muted">
          <p role="status">{currentState}</p>
          <dl className="grid gap-x-6 gap-y-2 sm:grid-cols-[auto_1fr]">
            {status?.latest === undefined ? null : <><dt>{t("about.latest")}</dt><dd className="text-ink">{status.latest}</dd></>}
            {status?.publishedAt === undefined ? null : <><dt>{t("about.publishedAt")}</dt><dd>{formatDateTime(status.publishedAt, locale)}</dd></>}
            <dt>{t("about.checkedAt")}</dt><dd>{status?.checkedAt === undefined ? t("about.notChecked") : formatDateTime(status.checkedAt, locale)}</dd>
          </dl>
          {status?.checkError ? <Notice tone="danger"><span className="grid gap-1"><span>{t("update.checkFailed")}</span><span className="break-words">{status.checkError}</span></span></Notice> : null}
          {reason === undefined ? null : <p>{t(updateMessage(reason))}</p>}
          <div className="flex flex-wrap items-baseline gap-4">
            <Button disabled={checking} onClick={() => { void checkUpdate(); }}>{t(checking ? "about.checking" : "about.check")}</Button>
            {status === null ? null : <UpdateControls status={status} api={updates} onStart={(job) => setStatus({ ...status, canUpdate: false, job })} />}
            {/* The sidebar's progress dialog stays closed here, so its reload button is offered on the page. */}
            {status?.job?.state === "restarting" ? <Button onClick={() => window.location.reload()}>{t("update.reload")}</Button> : null}
          </div>
          <UpdateActivityLog entries={activity.entries} />
        </div>
      </section>
      <ReleaseNotes releases={status?.releases ?? []} current={status?.current ?? about?.version ?? ""} latest={status?.latest} />
    </section>
  );
}
