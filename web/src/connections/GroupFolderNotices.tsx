import type { Notice } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { Button } from "../ui/surface";

const noticeText: Record<string, MessageKey> = {
  group_not_declared: "groupNotices.notDeclared",
  group_directory_missing: "groupNotices.directoryMissing",
  group_directory_leftover: "groupNotices.leftover",
  group_file_unreached: "groupNotices.fileUnreached",
};

// GroupFolderNotices は、グループの一覧の上に置く、フォルダとグループの食い違い（folderNoticesOf）である。
// 数が増えることがあるので「フォルダの確認 N件」に折りたたむ。Include行の無いフォルダは、その場で
// グループにできる（同じ名前のグループを作ると Include 行が書かれる）。
export function GroupFolderNotices({ notices, onDeclare }: { notices: readonly Notice[]; onDeclare: (name: string) => void }) {
  const t = useTranslate();
  if (notices.length === 0) return null;
  return (
    <details className="rounded-md border border-notice-line bg-notice px-2.5 py-1.5">
      <DisclosureSummary className="text-xs font-medium text-notice-ink">{t("groupNotices.summary", { count: notices.length })}</DisclosureSummary>
      <ul className="mt-2 flex flex-col gap-2.5 pb-1">
        {notices.map((notice, index) => (
          <li key={`${notice.code}-${notice.path ?? ""}-${index}`} className="flex flex-col items-start gap-1.5 text-xs leading-[18px] text-notice-ink">
            <span className="break-words">{t(noticeText[notice.code] ?? "groupNotices.fileUnreached", { path: notice.path ?? "", name: notice.detail ?? "" })}</span>
            {notice.code === "group_not_declared" && notice.detail !== undefined ? (
              <Button small onClick={() => onDeclare(notice.detail!)}>{t("groupNotices.declare")}</Button>
            ) : null}
          </li>
        ))}
      </ul>
    </details>
  );
}
