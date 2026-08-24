import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { sectionHeading } from "../ui/form";
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
      className="w-full shrink-0 bg-surface-subtle p-3 md:w-52 md:self-stretch md:border-r md:border-line"
    >
      <h3 className={`${sectionHeading} px-2 py-1 text-xs uppercase tracking-wider text-ink-muted`}>
        {t("keys.folders")}
      </h3>
      <ul className="mt-1 flex gap-1 overflow-x-auto md:flex-col md:overflow-visible">
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
                style={{ paddingLeft: `${0.5 + row.depth * 0.75}rem` }}
                className={`flex min-w-max items-center justify-between gap-3 rounded-lg py-2 pr-2 text-left text-sm md:w-full md:min-w-0 ${
                  current ? "bg-card font-semibold text-ink shadow-sm" : "text-ink-muted hover:bg-select-fill hover:text-ink"
                } ${dragging && target !== null ? "outline outline-1 outline-dashed outline-control-line" : ""} ${
                  over === key ? "bg-accent/20 outline-2 outline-solid outline-accent" : ""
                }`}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    aria-hidden="true"
                    className={`h-1.5 w-1.5 shrink-0 rounded-full ${current ? "bg-accent" : "bg-ink-faint"}`}
                  />
                  <span className="truncate">{labelOf(row.folder, t)}</span>
                </span>
                <span className="shrink-0 rounded-full bg-surface px-1.5 py-0.5 text-[11px] tabular-nums text-ink-muted">
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
