import { useTranslate } from "../i18n/context";
import { PreviewIconButton } from "./PreviewIconButton";

// PreviewStepper steps a file preview through the files around the shown one: count of them, the shown one at
// position. It draws nothing when there is only the one.
export function PreviewStepper({ position, count, onStep }: { position: number; count: number; onStep: (position: number) => void }) {
  const t = useTranslate();
  if (count < 2) return null;
  return (
    <>
      <PreviewIconButton icon="chevronLeft" label={t("sftp.previewPrevious")} disabled={position === 0} onClick={() => onStep(position - 1)} />
      <span className="shrink-0 whitespace-nowrap text-xs tabular-nums text-ink-muted">
        {t("sftp.previewPosition", { position: String(position + 1), count: String(count) })}
      </span>
      <PreviewIconButton icon="chevronRight" label={t("sftp.previewNext")} disabled={position === count - 1} onClick={() => onStep(position + 1)} />
    </>
  );
}
