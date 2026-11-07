import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";

type SelectionBarProps = {
  count: number;
  placement: "top" | "bottom";
  onConnect: () => void;
  // connectProblem は、選んだ接続先をまとめて開けない理由（セッションの上限など）。開けるなら undefined。
  connectProblem?: string | undefined;
  onMove: () => void;
  onDelete: () => void;
  onClear: () => void;
};

// SelectionBar は、選んだ接続先への操作を1行に並べる。
// 案Bでは一覧の上に、スマホでは一覧の下に置く。スマホでは下の丸いバーがこの帯の上に浮くので、帯の地をバーの下まで伸ばす。
// 右に接続先を開いた一覧の列や小さいスマホでは4つのボタンの文字が1行に入らないので、帯の幅が足りないボタンから
// 文字を隠してアイコンだけにし、件数も短く書く。名前はボタンの aria-label と title に残す。
export function SelectionBar({ count, placement, onConnect, connectProblem, onMove, onDelete, onClear }: SelectionBarProps) {
  const t = useTranslate();
  const frame = placement === "top"
    ? "border-b border-accent/40 bg-select-fill"
    : "sticky bottom-0 z-[2] border-t border-line bg-card pb-[calc(0.5rem+var(--mobile-navigation-inset))] shadow-[0_-4px_12px_rgb(0_0_0/0.08)]";
  const connectLabel = t("tree.connectAll");
  const moveLabel = t("tree.moveToGroup");
  return (
    <div className={`@container/selection shrink-0 ${frame}`}>
      <div role="toolbar" aria-label={t("tree.selectedCount", { count })} className="flex items-center gap-2 px-3 py-2">
        <span className="min-w-0 flex-1 truncate whitespace-nowrap text-sm font-medium text-ink">
          <span className="@xs/selection:hidden">{t("tree.selectedCountShort", { count })}</span>
          <span className="hidden @xs/selection:inline">{t("tree.selectedCount", { count })}</span>
        </span>
        <Button
          className="inline-flex items-center gap-1.5"
          disabled={count === 0 || connectProblem !== undefined}
          onClick={onConnect}
          aria-label={connectLabel}
          title={connectProblem ?? connectLabel}
        >
          <Icon name="terminal" className="size-4" />
          <span className="hidden @sm/selection:inline">{connectLabel}</span>
        </Button>
        <Button className="inline-flex items-center gap-1.5" disabled={count === 0} onClick={onMove} aria-label={moveLabel} title={moveLabel}>
          <Icon name="groups" className="size-4" />
          <span className="hidden @md/selection:inline">{moveLabel}</span>
        </Button>
        <Button kind="danger" className="inline-flex items-center gap-1.5" disabled={count === 0} onClick={onDelete} aria-label={t("tree.deleteEllipsis")} title={t("tree.deleteEllipsis")}>
          <Icon name="delete" className="size-4" />
        </Button>
        {placement === "top" ? (
          <button type="button" aria-label={t("tree.clearSelection")} onClick={onClear} className="flex size-8 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink">
            <Icon name="close" className="size-4" />
          </button>
        ) : null}
      </div>
    </div>
  );
}
