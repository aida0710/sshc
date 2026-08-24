import { useSyncExternalStore } from "react";
import { useTranslate } from "../i18n/context";
import { sftpTransferManager } from "./transferManager";

export function TransferNotifications() {
  const t = useTranslate();
  const notices = useSyncExternalStore(sftpTransferManager.subscribeNotices, sftpTransferManager.getNoticeSnapshot);
  if (notices.length === 0) return null;
  return (
    <aside className="fixed bottom-4 right-4 z-50 flex w-[min(22rem,calc(100vw-2rem))] flex-col gap-2" aria-label={t("sftp.notice.heading")} aria-live="polite">
      {notices.map((notice) => (
        <div key={notice.id} role={notice.status === "failed" ? "alert" : "status"} className={`rounded-lg border p-3 shadow-lg ${notice.status === "failed" ? "border-danger/40 bg-card text-danger" : "border-live/40 bg-card text-ink"}`}>
          <div className="flex items-start gap-2">
            <p className="min-w-0 grow text-sm">{t(notice.status === "completed" ? "sftp.notice.completed" : "sftp.notice.failed", { name: notice.name, direction: t(notice.direction === "upload" ? "sftp.manager.upload" : "sftp.manager.download"), problem: notice.problem })}</p>
            <button type="button" className="text-xs text-ink-muted" aria-label={t("sftp.notice.dismiss")} onClick={() => sftpTransferManager.dismissNotice(notice.id)}>×</button>
          </div>
        </div>
      ))}
    </aside>
  );
}
