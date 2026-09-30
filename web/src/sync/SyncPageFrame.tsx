import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { PageHeader } from "../ui/page";

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

// SyncPageFrame は、Sync の画面のどの状態（失敗、Vault のロック、通常）にも共通の
// 枠と見出しである。
export function SyncPageFrame({ children }: { children: ReactNode }) {
  const t = useTranslate();
  return (
    <div className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}>
      <PageHeader title={t("sync.heading")} description={t("sync.pageDescription")} />
      {children}
    </div>
  );
}
