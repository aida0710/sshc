import { useState } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import { dropTargetLook } from "../ui/dragAndDropLook";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { sameFolder, type Folder, type FolderRow, type MoveTarget } from "./organizer";

type FolderPaneProps = {
  rows: FolderRow[];
  selected: Folder;
  dragging: boolean;
  onSelect: (folder: Folder) => void;
  onDropInto: (target: MoveTarget) => void;
};

function labelOf(folder: Folder, t: Translate): string {
  if (folder.kind === "all") return t("keys.folderAll");
  if (folder.kind === "ungrouped") return t("keys.folderUngrouped");
  if (folder.kind === "trash") return t("keys.folderTrash");
  return folder.name;
}

function keyOf(folder: Folder): string {
  return `${folder.kind}:${folder.kind === "group" ? folder.name : ""}`;
}

function dropTargetOf(folder: Folder): MoveTarget | null {
  return folder.kind === "ungrouped" || folder.kind === "group" ? folder : null;
}

// FolderPane is the left list of the keys page: every key, each group, the keys outside a group, and the
// trash, with how many rows each shows. Keys dragged from the table can be dropped on a group. Where the
// list has no room beside the table (the keylist container is narrow, as on a phone), it is a select.
export function FolderPane({ rows, selected, dragging, onSelect, onDropInto }: FolderPaneProps) {
  const t = useTranslate();
  const [over, setOver] = useState("");

  return (
    <nav aria-label={t("keys.foldersLabel")} className="w-full shrink-0 @3xl/keylist:w-[188px]">
      <label className="block @3xl/keylist:hidden">
        <span className="sr-only">{t("keys.folders")}</span>
        <select
          className={control}
          value={keyOf(selected)}
          onChange={(event) => {
            const row = rows.find((candidate) => keyOf(candidate.folder) === event.target.value);
            if (row !== undefined) onSelect(row.folder);
          }}
        >
          {rows.map((row) => (
            <option key={keyOf(row.folder)} value={keyOf(row.folder)}>
              {"　".repeat(Math.max(0, row.depth - 1)) + t("keys.folderOption", { name: labelOf(row.folder, t), count: row.count })}
            </option>
          ))}
        </select>
      </label>
      <div className="hidden @3xl/keylist:block">
        <h4 className="px-2.5 pb-1.5 text-xs text-ink-faint">{t("keys.folders")}</h4>
        <ul className="flex flex-col gap-0.5">
          {rows.map((row) => {
            const target = dropTargetOf(row.folder);
            const current = sameFolder(row.folder, selected);
            const key = keyOf(row.folder);
            const trash = row.folder.kind === "trash";
            return (
              <li key={key} className={trash ? "mt-1.5" : undefined}>
                <button
                  type="button"
                  aria-label={t("keys.folderRow", { name: labelOf(row.folder, t), count: row.count })}
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
                  className={`flex min-h-[30px] w-full items-center gap-2 rounded-[7px] pr-2.5 text-left text-[13px] ${
                    current ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-hover hover:text-ink"
                  } ${dropTargetLook({ accepts: dragging && target !== null, over: over === key })}`}
                >
                  {trash ? <Icon name="delete" className="size-3.5 shrink-0" /> : null}
                  <span className="min-w-0 flex-1 truncate">{labelOf(row.folder, t)}</span>
                  <span className="shrink-0 font-mono text-[11.5px] tabular-nums text-ink-faint">{row.count}</span>
                </button>
              </li>
            );
          })}
        </ul>
      </div>
    </nav>
  );
}
