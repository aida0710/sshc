import { useTranslate } from "../i18n/context";
import { sectionHeading } from "../ui/form";
import { sameFolder, type Folder, type FolderRow, type MoveTarget } from "./organizer";

type FolderPaneProps = {
  rows: FolderRow[];
  selected: Folder;
  // dragging は、いま鍵をつかんでいるかどうか。掴んでいないときに置き場を
  // 光らせても、そこへ何かを置けるという嘘になる。
  dragging: boolean;
  onSelect: (folder: Folder) => void;
  onDropInto: (target: MoveTarget) => void;
};

function labelOf(folder: Folder, t: ReturnType<typeof useTranslate>): string {
  if (folder.kind === "all") return t("keys.folderAll");
  if (folder.kind === "ungrouped") return t("keys.folderUngrouped");
  return folder.name;
}

// dropTargetOf は、そのフォルダが置き場かどうかを答える。
//
// **「すべて」は置き場ではない。** あれは絞り込みを外すことであって、
// ~/.ssh の中の実在の場所ではない。放れてしまうと、利用者は鍵をどこかへ
// 移したつもりになるが、移った先が無い。
function dropTargetOf(folder: Folder): MoveTarget | null {
  return folder.kind === "all" ? null : folder;
}

export function FolderPane({ rows, selected, dragging, onSelect, onDropInto }: FolderPaneProps) {
  const t = useTranslate();

  return (
    <nav aria-label={t("keys.foldersLabel")} className="w-56 shrink-0">
      <h3 className={sectionHeading}>{t("keys.folders")}</h3>
      <ul className="mt-2 flex flex-col gap-0.5">
        {rows.map((row) => {
          const target = dropTargetOf(row.folder);
          const current = sameFolder(row.folder, selected);
          // 名前と件数は別の要素なので、そのままでは "All keys3" と読み上げ
          // られる。読める形をここで与える。
          const spoken = t("keys.folderRow", { name: labelOf(row.folder, t), count: row.count });
          return (
            <li key={`${row.folder.kind}:${row.folder.kind === "group" ? row.folder.name : ""}`}>
              <button
                type="button"
                aria-label={spoken}
                aria-current={current}
                onClick={() => onSelect(row.folder)}
                onDragOver={target === null ? undefined : (event) => event.preventDefault()}
                onDrop={
                  target === null
                    ? undefined
                    : (event) => {
                        event.preventDefault();
                        onDropInto(target);
                      }
                }
                style={{ paddingLeft: `${0.5 + row.depth * 0.75}rem` }}
                className={`flex w-full items-center justify-between rounded py-1 pr-2 text-left text-sm ${
                  current ? "bg-control font-semibold text-ink" : "text-ink-muted hover:bg-select-fill"
                } ${dragging && target !== null ? "outline outline-1 outline-dashed outline-control-line" : ""}`}
              >
                <span className="truncate">{labelOf(row.folder, t)}</span>
                <span className="ml-2 shrink-0 text-xs tabular-nums text-ink-muted">{row.count}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
