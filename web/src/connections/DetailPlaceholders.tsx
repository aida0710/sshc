import { Button } from "../ui/surface";
import { Icon } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";


export function MissingConnection({ onBackToList }: { onBackToList: () => void }) {
  const t = useTranslate();
  return (
    <section className="m-auto flex max-w-sm flex-col items-center rounded-lg bg-tree px-8 py-10 text-center" role="status">
      <h2 className="text-lg font-semibold text-ink">{t("conn.missingHeading")}</h2>
      <p className="mt-1 text-sm leading-6 text-ink-muted">{t("conn.missingHint")}</p>
      <Button kind="primary" className="mt-4" onClick={onBackToList}>
        {t("conn.backToList")}
      </Button>
    </section>
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
    <section className="m-auto flex max-w-sm flex-col items-center rounded-lg bg-tree px-8 py-10 text-center" role="status">
      <span
        aria-hidden="true"
        className="mb-4 flex size-14 items-center justify-center rounded-lg bg-card text-accent shadow-sm"
      >
        <Icon name="connections" className="size-7" />
      </span>
      <h2 className="text-lg font-semibold text-ink">
        {t(preferredKey === null ? "conn.emptyHeading" : "conn.assignKeyHeading")}
      </h2>
      <p className="mt-1 text-sm leading-6 text-ink-muted">
        {preferredKey === null
          ? t("conn.emptyHint")
          : t("conn.assignKeyHint", { path: preferredKey.privateRelativePath })}
      </p>
      <Button kind="primary" className="mt-4" onClick={onBeginCreation}>
        {t("conn.createAnother")}
      </Button>
    </section>
  );
}
