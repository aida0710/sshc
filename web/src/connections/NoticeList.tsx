import type { Notice } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { DisclosureSummary } from "../ui/DisclosureSummary";

const noticeKeys: Record<string, MessageKey> = {
  complex_external_rule: "notice.complex_external_rule",
  duplicate_alias: "notice.duplicate_alias",
  proxy_ignored: "notice.proxy_ignored",
  wildcard_shadow: "notice.wildcard_shadow",
  negated_pattern: "notice.negated_pattern",
  unnamed_host_block: "notice.unnamed_host_block",
  match_block: "notice.match_block",
  dangerous_directive: "notice.dangerous_directive",
  unstructured_line: "notice.unstructured_line",
  external_file: "notice.external_file",
  orphan_metadata: "notice.orphan_metadata",
  group_cycle: "notice.group_cycle",
  group_member_missing: "notice.group_member_missing",
  explained_values_only: "notice.explained_values_only",
  match_exec_refused: "notice.match_exec_refused",
  match_final_refused: "notice.match_final_refused",
  canonicalise_refused: "notice.canonicalise_refused",
  unknown_token_refused: "notice.unknown_token_refused",
  destination_not_included: "notice.destination_not_included",
  include_no_longer_matches: "notice.include_no_longer_matches",
  include_not_rewritten: "notice.include_not_rewritten",
  include_now_unreached: "notice.include_now_unreached",
  group_file_unreached: "notice.group_file_unreached",
  group_directory_leftover: "notice.group_directory_leftover",
  group_not_declared: "notice.group_not_declared",
  group_directory_missing: "notice.group_directory_missing",
  group_empty: "notice.group_empty",
  generated_region_damaged: "notice.generated_region_damaged",
};

function locationText(location: { path?: string; line?: number }): string {
  return `${location.path ?? ""}${location.line === undefined ? "" : `:${location.line}`}`;
}

// NoticeText は、注意の1件を、何がどこで起きているかが分かる文にする。
function NoticeText({ notice }: { notice: Notice }) {
  const t = useTranslate();
  if (notice.code === "duplicate_alias" && notice.detail !== undefined && notice.related !== undefined && notice.path !== undefined) {
    return <>{t("notice.duplicate_alias_both", { alias: notice.detail, first: locationText(notice.related), second: locationText(notice) })}</>;
  }
  return (
    <>
      {notice.code in noticeKeys ? t(noticeKeys[notice.code]!) : notice.code}
      {notice.path === undefined ? "" : ` (${locationText(notice)})`}
    </>
  );
}

// NoticeList は、設定についての注意を並べる。collapsible なら「警告 N件」の行に折りたたみ、
// 押したときだけ中身を見せる（同じ種類の注意が接続先の数だけ並ぶことがあるため）。
export function NoticeList({ notices, collapsible = false }: { notices: Notice[]; collapsible?: boolean }) {
  const t = useTranslate();
  if (notices.length === 0) return null;
  const list = (
    <ul className="flex flex-col gap-1">
      {notices.map((notice, index) => (
        <li key={`${notice.code}-${notice.path ?? ""}-${notice.line ?? 0}-${index}`} className="text-xs text-notice-ink">
          <NoticeText notice={notice} />
        </li>
      ))}
    </ul>
  );
  if (!collapsible) return list;
  return (
    <details className="rounded-md border border-notice-line bg-notice px-3 py-2">
      <DisclosureSummary className="text-xs font-medium text-notice-ink">{t("notice.summary", { count: notices.length })}</DisclosureSummary>
      <div className="mt-2">{list}</div>
    </details>
  );
}
