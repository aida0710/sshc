import { useState } from "react";
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
  // **いまどこへ落ちるのかを見せる。** 候補が全部同じ見た目だと、放す瞬間まで
  // 行き先が分からない。掴んでいる手は、狙いが合っているかを確かめられない。
  const [over, setOver] = useState("");

  return (
    // **貼り付ける。** 一覧が長いと、鍵まで下ろした時点でフォルダは画面の外に
    // ある——運ぶ相手が見えていないのだから、ドラッグは成立しない。
    <nav aria-label={t("keys.foldersLabel")} className="w-56 shrink-0 self-start md:sticky md:top-4">
      <h3 className={sectionHeading}>{t("keys.folders")}</h3>
      <ul className="mt-2 flex flex-col gap-0.5">
        {rows.map((row) => {
          const target = dropTargetOf(row.folder);
          const current = sameFolder(row.folder, selected);
          // 名前と件数は別の要素なので、そのままでは "All keys3" と読み上げ
          // られる。読める形をここで与える。
          const spoken = t("keys.folderRow", { name: labelOf(row.folder, t), count: row.count });
          const key = `${row.folder.kind}:${row.folder.kind === "group" ? row.folder.name : ""}`;
          return (
            <li key={key}>
              <button
                type="button"
                aria-label={spoken}
                aria-current={current}
                onClick={() => onSelect(row.folder)}
                onDragOver={
                  target === null
                    ? undefined
                    : (event) => {
                        event.preventDefault();
                        setOver(key);
                      }
                }
                onDragLeave={target === null ? undefined : () => setOver((current) => (current === key ? "" : current))}
                onDrop={
                  target === null
                    ? undefined
                    : (event) => {
                        event.preventDefault();
                        setOver("");
                        onDropInto(target);
                      }
                }
                style={{ paddingLeft: `${0.5 + row.depth * 0.75}rem` }}
                className={`flex w-full items-center justify-between rounded py-2 pr-2 text-left text-sm ${
                  current ? "bg-control font-semibold text-ink" : "text-ink-muted hover:bg-select-fill"
                } ${dragging && target !== null ? "outline outline-1 outline-dashed outline-control-line" : ""} ${
                  over === key ? "bg-accent/20 outline-2 outline-solid outline-accent" : ""
                }`}
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
