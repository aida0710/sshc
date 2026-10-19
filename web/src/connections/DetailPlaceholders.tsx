import { Button } from "../ui/surface";
import { EmptyState } from "../ui/EmptyState";
import { useTranslate } from "../i18n/context";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";

export function MissingConnection({ onBackToList }: { onBackToList: () => void }) {
  const t = useTranslate();
  return (
    <EmptyState
      icon="connections"
      title={t("conn.missingHeading")}
      detail={t("conn.missingHint")}
      action={<Button kind="primary" onClick={onBackToList}>{t("conn.backToList")}</Button>}
    />
  );
}

export function NoConnectionSelected({
  preferredKey,
  onBeginCreation,
}: {
  preferredKey: GeneratedPrivateKeyHandoff | null;
  onBeginCreation: () => void;
}) {
  const t = useTranslate();
  return (
    <EmptyState
      icon="connections"
      title={t(preferredKey === null ? "conn.emptyHeading" : "conn.assignKeyHeading")}
      detail={preferredKey === null
        ? t("conn.emptyHint")
        : t("conn.assignKeyHint", { path: preferredKey.privateRelativePath })}
      action={<Button kind="primary" onClick={onBeginCreation}>{t("conn.createAnother")}</Button>}
    />
  );
}
