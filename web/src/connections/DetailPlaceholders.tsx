import { Button } from "../ui/surface";
import { Icon } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";

// 詳細のペインが、開く相手を持たないときに出すものである。
//
// **どちらも空白ではない。** URL が名指した接続が設定から消えていることと、
// まだ何も選ばれていないことは別の出来事で、次にすべきことも違う。

// MissingConnection は、URL が名指したホストが設定に無いことを言う。
//
// **空の詳細を出さない。** 名前が消えたのか、まだ読み込んでいないのかは、
// 見る人にとって別のことである。
export function MissingConnection({ onBackToList }: { onBackToList: () => void }) {
  const t = useTranslate();
  return (
    <section className="m-auto flex max-w-sm flex-col items-center text-center" role="status">
      <h2 className="text-lg font-semibold text-ink">{t("conn.missingHeading")}</h2>
      <p className="mt-1 text-sm leading-6 text-ink-muted">{t("conn.missingHint")}</p>
      <Button kind="primary" className="mt-4" onClick={onBackToList}>
        {t("conn.backToList")}
      </Button>
    </section>
  );
}

// NoConnectionSelected は、まだ何も選ばれていないときの面である。
//
// **鍵を持って来た人には、違うことを言う。** 鍵を作った直後にここへ来たなら、
// 次にすることは「眺める」ではなく「その鍵を使う接続を作る」である。
export function NoConnectionSelected({
  preferredKey,
  onBeginCreation,
}: {
  preferredKey: GeneratedPrivateKeyHandoff | null;
  onBeginCreation: () => void;
}) {
  const t = useTranslate();
  return (
    <section className="m-auto flex max-w-sm flex-col items-center text-center" role="status">
      <span
        aria-hidden="true"
        className="mb-4 flex size-14 items-center justify-center rounded-2xl border border-line bg-card text-ink-muted shadow-sm"
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
