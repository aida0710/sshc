import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "./ConfirmDialog";

// DiscardDraftDialog は、未保存の下書きを捨ててよいかを確かめる（useDraftDiscardConfirmation と組で使う）。
export function DiscardDraftDialog({ id, onConfirm, onCancel }: { id: string; onConfirm: () => void; onCancel: () => void }) {
  const t = useTranslate();
  return (
    <ConfirmDialog
      id={id}
      heading={t("draft.discardHeading")}
      body={<p className="text-sm text-ink-muted">{t("draft.discardBody")}</p>}
      confirmLabel={t("draft.discardConfirm")}
      cancelLabel={t("draft.keepEditing")}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
