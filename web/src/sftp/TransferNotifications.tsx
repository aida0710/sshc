import { useEffect, useRef, useSyncExternalStore } from "react";
import { Icon } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import { showBrowserNotification } from "../ui/browserNotifications";
import { sftpTransferManager, type TransferNotice } from "./transferManager";
import { sftpTransferProblemText } from "./sftpProblemText";

export function notifyBackgroundTransfers(
  notices: readonly TransferNotice[],
  delivered: Set<string>,
  t: ReturnType<typeof useTranslate>,
  hidden = document.hidden,
): void {
  for (const notice of notices) {
    if (delivered.has(notice.id)) continue;
    delivered.add(notice.id);
    if (!hidden) continue;
    showBrowserNotification({
      title: "sshc",
      body: t(notice.status === "completed" ? "sftp.notice.completed" : "sftp.notice.failed", {
        name: notice.name,
        direction: t(notice.direction === "upload" ? "sftp.manager.upload" : "sftp.manager.download"),
        problem: sftpTransferProblemText(t, notice.problem),
      }),
      tag: `sshc-transfer-${notice.jobId}`,
    });
  }
}

export function TransferNotifications() {
  const t = useTranslate();
  const notices = useSyncExternalStore(sftpTransferManager.subscribeNotices, sftpTransferManager.getNoticeSnapshot);
  const delivered = useRef(new Set<string>());
  useEffect(() => {
    notifyBackgroundTransfers(notices, delivered.current, t);
  }, [notices, t]);
  if (notices.length === 0) return null;
  return (
    <aside
      className="pointer-events-none fixed right-4 top-20 z-50 flex w-[min(22rem,calc(100vw-2rem))] flex-col gap-2 md:bottom-4 md:top-auto"
      aria-label={t("sftp.notice.heading")}
      aria-live="polite"
    >
      {notices.map((notice) => {
        const failed = notice.status === "failed";
        const message = t(notice.status === "completed" ? "sftp.notice.completed" : "sftp.notice.failed", {
          name: notice.name,
          direction: t(notice.direction === "upload" ? "sftp.manager.upload" : "sftp.manager.download"),
          problem: sftpTransferProblemText(t, notice.problem),
        });
        return (
          <div
            key={notice.id}
            role={failed ? "alert" : "status"}
            className={`rounded-lg border p-3 shadow-lg ${failed ? "border-danger/40 bg-card text-danger" : "border-live/40 bg-card text-ink"}`}
          >
            <div className="flex items-start gap-2">
              <p className="min-w-0 grow text-sm">{message}</p>
              <button
                type="button"
                className="pointer-events-auto text-xs text-ink-muted"
                aria-label={t("sftp.notice.dismiss")}
                onClick={() => sftpTransferManager.dismissNotice(notice.id)}
              >
                <Icon name="close" className="size-3.5" />
              </button>
            </div>
          </div>
        );
      })}
    </aside>
  );
}
