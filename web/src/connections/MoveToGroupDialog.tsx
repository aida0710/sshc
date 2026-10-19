import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import type { GroupListing } from "./groupListing";

type MoveToGroupDialogProps = {
  count: number;
  groups: readonly GroupListing[];
  // 選んだ接続先がすべているグループ。そこへは移せないので選べなくする。
  currentGroup: string | null;
  onPick: (group: string) => void;
  onCancel: () => void;
};

// MoveToGroupDialog は、選んだ接続先の移動先を、グループの一覧から1回のタップで選ばせる。
// スマホでは下から出る。
export function MoveToGroupDialog({ count, groups, currentGroup, onPick, onCancel }: MoveToGroupDialogProps) {
  const t = useTranslate();
  const rows = [
    ...groups.map((group) => ({ name: group.name, label: group.label, depth: group.depth })),
    { name: "", label: t("tree.ungrouped"), depth: 0 },
  ];
  return (
    <ModalShell
      labelledBy="move-to-group-heading"
      onDismiss={onCancel}
      closeOnOutside
      placement="sheet"
      panelClassName="flex max-h-[80dvh] w-full max-w-md flex-col rounded-lg"
    >
      <h2 id="move-to-group-heading" className="border-b border-line px-4 py-3 text-base font-semibold text-ink">
        {t("moveDialog.heading", { count })}
      </h2>
      <ul className="min-h-0 flex-1 overflow-y-auto p-2">
        {rows.map((row) => (
          <li key={row.name || "ungrouped"}>
            <button
              type="button"
              disabled={row.name === currentGroup}
              onClick={() => onPick(row.name)}
              style={{ paddingInlineStart: `${0.75 + row.depth * 1.25}rem` }}
              className="flex min-h-11 w-full items-center gap-2 rounded-md pr-3 text-left text-sm text-ink hover:bg-select-fill disabled:text-ink-faint disabled:hover:bg-transparent md:min-h-9"
            >
              <Icon name="groups" className="size-4 text-ink-faint" />
              <span className="min-w-0 flex-1 truncate">{row.label}</span>
            </button>
          </li>
        ))}
      </ul>
      <div className="flex justify-end border-t border-line px-4 py-3">
        <Button onClick={onCancel}>{t("groupDialog.cancel")}</Button>
      </div>
    </ModalShell>
  );
}
