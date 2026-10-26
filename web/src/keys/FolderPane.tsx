import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { dropTargetLook } from "../ui/dragAndDropLook";
import { sameFolder, type Folder, type FolderRow, type MoveTarget } from "./organizer";

type FolderPaneProps = {
  rows: FolderRow[];
  selected: Folder;
  dragging: boolean;
  onSelect: (folder: Folder) => void;
  onDropInto: (target: MoveTarget) => void;
};

function labelOf(folder: Folder, t: ReturnType<typeof useTranslate>): string {
  if (folder.kind === "all") return t("keys.folderAll");
  if (folder.kind === "ungrouped") return t("keys.folderUngrouped");
  return folder.name;
}

function dropTargetOf(folder: Folder): MoveTarget | null {
  return folder.kind === "all" ? null : folder;
}

export function FolderPane({ rows, selected, dragging, onSelect, onDropInto }: FolderPaneProps) {
  const t = useTranslate();
  const [over, setOver] = useState("");

  return (
    <nav
      aria-label={t("keys.foldersLabel")}
      className="w-full shrink-0 md:w-[188px]"
    >
      <h4 className="px-2.5 pb-1.5 text-xs text-ink-faint">
        {t("keys.folders")}
      </h4>
      <ul className="flex gap-0.5 overflow-x-auto md:flex-col md:overflow-visible">
        {rows.map((row) => {
          const target = dropTargetOf(row.folder);
          const current = sameFolder(row.folder, selected);
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
                style={{ paddingLeft: `${0.625 + row.depth * 0.75}rem` }}
                className={`flex min-h-[30px] min-w-max items-center justify-between gap-3 rounded-[7px] pr-2.5 text-left text-[13px] md:w-full md:min-w-0 ${
                  current ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-hover hover:text-ink"
                } ${dropTargetLook({ accepts: dragging && target !== null, over: over === key })}`}
              >
                <span className="truncate">{labelOf(row.folder, t)}</span>
                <span className="shrink-0 font-mono text-[11.5px] tabular-nums text-ink-faint">
                  {row.count}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
