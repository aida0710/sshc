import { useTranslate } from "../i18n/context";
import type { KeyInventoryResponse } from "./api";

// KeyInventoryProblems は、種類を判定できなかったファイルと、設定が指している
// のに見つからない鍵を並べる。一覧から黙って外さないためである。
export function KeyInventoryProblems({ inventory }: { inventory: KeyInventoryResponse }) {
  const t = useTranslate();
  return (
    <>
      {inventory.unreadable.length > 0 && (
        <section aria-labelledby="unreadable-heading" className="flex flex-col gap-2">
          <h3 id="unreadable-heading" className="text-sm font-medium text-notice-ink">
            {t("keys.unreadableHeading")}
          </h3>
          <p className="text-sm text-ink-muted">{t("keys.unreadableNote")}</p>
          <ul className="text-sm text-ink-muted">
            {inventory.unreadable.map((file) => (
              <li key={file.relativePath}>
                {t("keys.unreadableEntry", { path: file.relativePath, reason: file.reason })}
              </li>
            ))}
          </ul>
        </section>
      )}
      {inventory.unresolvedReferences.length > 0 && (
        <section aria-labelledby="unresolved-heading" className="flex flex-col gap-2">
          <h3 id="unresolved-heading" className="text-sm font-medium text-notice-ink">
            {t("keys.unresolvedHeading")}
          </h3>
          <ul className="text-sm text-ink-muted">
            {inventory.unresolvedReferences.map((reference) => (
              <li key={`${reference.configPath}:${reference.line}:${reference.value}`}>
                {t("keys.referenceWithReason", {
                  directive: reference.directive,
                  value: reference.value,
                  path: reference.configPath,
                  line: reference.line,
                  reason: reference.reason,
                })}
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}
