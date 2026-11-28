import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { IconButton } from "../../ui/IconButton";

// PaneToolbar は、分割表示のペインの見出しの右端のボタンである。入れ替えと集中表示はふだん使わないので［…］に入れ、
// 見出しには［×］だけを置く。集中表示の間は戻し方が分かるよう、［集中表示をやめる］も見えるまま出す。
export function PaneToolbar({ title, focusMode, onFocusMode, onRemove }: {
  title: string;
  focusMode: boolean;
  onFocusMode: () => void;
  onRemove: () => void;
}) {
  const t = useTranslate();
  // セッションは分割表示の外で続くので、閉じるとは言わない。
  const removeLabel = t("splitView.removePane", { title });
  return (
    <div className="flex shrink-0 items-center gap-0.5" data-pane-toolbar>
      {focusMode ? (
        <IconButton label={t("splitView.exitFocus")} size="toolbar" tone="faint" onClick={onFocusMode}>
          <Icon name="focus" className="size-[15px]" />
        </IconButton>
      ) : null}
      <IconButton label={removeLabel} size="toolbar" tone="faint" onClick={onRemove}>
        <Icon name="close" className="size-[15px]" />
      </IconButton>
    </div>
  );
}
