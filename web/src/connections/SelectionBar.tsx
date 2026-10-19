import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";

type SelectionBarProps = {
  count: number;
  placement: "top" | "bottom";
  onMove: () => void;
  onDelete: () => void;
  onClear: () => void;
};

// SelectionBar は、選んだ接続先への操作を1行に並べる。
// 案Bでは一覧の上に、スマホでは一覧の下に置く。
export function SelectionBar({ count, placement, onMove, onDelete, onClear }: SelectionBarProps) {
  const t = useTranslate();
  const frame = placement === "top"
    ? "border-b border-accent/40 bg-select-fill"
    : "sticky bottom-0 z-[2] border-t border-line bg-card pb-[max(0.5rem,env(safe-area-inset-bottom))] shadow-[0_-4px_12px_rgb(0_0_0/0.08)]";
  return (
    <div role="toolbar" aria-label={t("tree.selectedCount", { count })} className={`flex shrink-0 items-center gap-2 px-3 py-2 ${frame}`}>
      <span className="min-w-0 flex-1 whitespace-nowrap text-sm font-medium text-ink">{t("tree.selectedCount", { count })}</span>
      <Button className="inline-flex items-center gap-1.5" disabled={count === 0} onClick={onMove}>
        <Icon name="groups" className="size-4" />
        {t("tree.moveToGroup")}
      </Button>
      <Button kind="danger" className="inline-flex items-center gap-1.5" disabled={count === 0} onClick={onDelete} aria-label={t("tree.deleteEllipsis")} title={t("tree.deleteEllipsis")}>
        <Icon name="delete" className="size-4" />
      </Button>
      {placement === "top" ? (
        <button type="button" aria-label={t("tree.clearSelection")} onClick={onClear} className="flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink">
          <Icon name="close" className="size-4" />
        </button>
      ) : null}
    </div>
  );
}
