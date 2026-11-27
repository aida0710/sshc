import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AboutApi } from "../api/about";
import { runTargetTerminalSessionsApi, terminalSessionsApi, type TerminalSessionsApi } from "../api/terminalSessions";
import type { UpdateApi } from "../api/update";
import { useEngineApis } from "../shell/engineApis";
import { useLanguage } from "../i18n/context";
import { UpdateControls } from "../shell/UpdateControls";
import { explainedUpdateReason, updateMessage } from "../shell/updateMessage";
import { ReleasePageLink } from "./ReleasePageLink";
import { isUpdateActive, useUpdateStatus } from "../shell/useUpdateStatus";
import { formatDateTime } from "../ui/format";
import { FactRows, type Fact } from "../ui/FactRows";
import { PageHeader, PageSection } from "../ui/page";
import { Button, Notice } from "../ui/surface";
import { useLoad } from "../ui/useLoad";
import { BuildDetails } from "./BuildDetails";
import { ReleaseNotes } from "./ReleaseNotes";
import { checkRequestFailedActivity, checkResultActivity, checkStartedActivity } from "./updateActivity";
import { UpdateActivityLog } from "./UpdateActivityLog";
import { UpdateOutput } from "./UpdateOutput";
import { useUpdateActivity } from "./useUpdateActivity";
import { useUpdateOutput } from "./useUpdateOutput";
import { PanelState } from "../ui/PanelState";

type AboutPageProps = { api?: AboutApi; updates?: UpdateApi; sessions?: Pick<TerminalSessionsApi, "terminalSessions"> };

export function AboutPage({ api: givenApi, updates: givenUpdates, sessions: givenSessions }: AboutPageProps) {
  const { t, locale } = useLanguage();
  // バージョンと更新は、選んでいる sshc エンジン（リモートセッション）のもの。更新で終わるセッションも、そのエンジンのもの。
  const engine = useEngineApis();
  const api = givenApi ?? engine.about;
  const updates = givenUpdates ?? engine.update;
  const engineSessions = useMemo(() => engine.runTarget === null ? terminalSessionsApi : runTargetTerminalSessionsApi(engine.runTarget), [engine.runTarget]);
  const sessions = givenSessions ?? engineSessions;
  const aboutLoad = useLoad(useCallback(() => api.about(), [api]));
  const about = aboutLoad.state === "loaded" ? aboutLoad.value : null;
  const [checking, setChecking] = useState(false);
  // A stored result may be days old; only a successful check on this page
  // can establish whether a newer version exists.
  const [checkedOnThisPage, setCheckedOnThisPage] = useState(false);
  // The status also carries the failure of the engine's own last check, which may be hours old and says nothing about
  // the network now. Only a check made on this page is reported as failed.
  const [checkFailure, setCheckFailure] = useState("");
  const openingCheckHandled = useRef(false);
  const { status, setStatus } = useUpdateStatus(updates);
  const activity = useUpdateActivity(status);
  const updateOutput = useUpdateOutput(updates, status?.job);

  async function checkUpdate() {
    if (checking) return;
    openingCheckHandled.current = true;
    setChecking(true);
    setCheckedOnThisPage(false);
    setCheckFailure("");
    activity.record(checkStartedActivity);
    try {
      const checked = await updates.checkUpdate();
      setStatus(checked);
      setCheckedOnThisPage(!checked.checkError);
      setCheckFailure(checked.checkError ?? "");
      activity.record(checkResultActivity(checked));
    } catch {
      activity.record(checkRequestFailedActivity);
    } finally {
      setChecking(false);
    }
  }

  // Wait for the job status so an opening check cannot interfere with update
  // progress. The ref also prevents StrictMode from checking twice.
  useEffect(() => {
    if (openingCheckHandled.current || status === null) return;
    openingCheckHandled.current = true;
    if (isUpdateActive(status.job)) return;
    void checkUpdate();
  });

  const reason = status === null ? undefined : explainedUpdateReason(status);
  const currentState = getUpdateStateMessage();

  function getUpdateStateMessage(): string | null {
    // 確かめている間は［更新を確認］が「確認中…」と言うので、ここには書かない。
    if (!checkedOnThisPage || status === null) return null;
    if (status.checkError) return null;
    if (status.latest === undefined) return t("about.noRelease");
    return status.available ? t("about.available", { version: status.latest }) : t("about.noNewerVersion", { version: status.current });
  }

  const updateFacts: Fact[] = [];
  if (status?.latest !== undefined) updateFacts.push({ label: t("about.latest"), value: status.latest });
  if (status?.publishedAt !== undefined) updateFacts.push({ label: t("about.publishedAt"), value: formatDateTime(status.publishedAt, locale) });
  updateFacts.push({ label: t("about.checkedAt"), value: status?.checkedAt === undefined ? t("about.notChecked") : formatDateTime(status.checkedAt, locale) });

  return (
    <section aria-labelledby="about-heading" className="flex flex-col gap-[22px]">
      <PageHeader title={t("section.about")} headingId="about-heading" />
      <PageSection heading={t("about.build")}>
        {aboutLoad.state === "failed" ? <Notice tone="danger">{t("about.loadFailed")}</Notice> : about === null ? <PanelState layout="line" tone="loading" title={t("about.loading")} /> : <BuildDetails about={about} />}
      </PageSection>
      <PageSection heading={t("about.updates")} headingId="about-update-heading">
        <div className="grid grid-cols-[minmax(0,1fr)] gap-3 text-sm text-ink-muted">
          {currentState === null ? null : <p role="status" className="[overflow-wrap:anywhere]">{currentState}</p>}
          <FactRows facts={updateFacts} />
          {checkFailure === "" ? null : <Notice tone="danger">{t("update.checkFailed")}<span className="mt-1 block text-ink-muted">{checkFailure}</span></Notice>}
          {reason === undefined ? null : <p>{t(updateMessage(reason))}</p>}
          <div className="flex flex-wrap items-baseline gap-4">
            <Button busy={checking} busyLabel={t("busy.checking")} onClick={() => { void checkUpdate(); }}>{t("about.check")}</Button>
            {status === null ? null : <UpdateControls status={status} api={updates} sessions={sessions} onStart={(job) => setStatus({ ...status, canUpdate: false, job })} />}
            {status === null ? null : <ReleasePageLink status={status} />}
            {/* The sidebar's progress dialog stays closed here, so its reload button is offered on the page. */}
            {status?.job?.state === "restarting" ? <Button onClick={() => window.location.reload()}>{t("update.reload")}</Button> : null}
          </div>
          <UpdateActivityLog entries={activity.entries} />
          <UpdateOutput {...updateOutput} />
        </div>
      </PageSection>
      <ReleaseNotes releases={status?.releases ?? []} current={status?.current ?? about?.version ?? ""} latest={status?.latest} />
    </section>
  );
}
