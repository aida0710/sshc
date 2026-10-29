import { Fragment, type ReactNode } from "react";
import type { FileNode } from "../api/config";
import { useTranslate, type Translate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { configTreeRows } from "./configTree";

// 一段深くなるごとの字下げと、Include の行をファイル名の頭にそろえる字下げ（試作の Include階層と同じ）。
const depthIndentPx = 14;
const includeIndentPx = 26;

// fileNotes は、ファイルの行の下に出す、知っておくべき状態（無い・何度も読まれる・書き込めない）である。
// 編集できるのは普通のことなので書かない。
function fileNotes(t: Translate, node: FileNode): string {
  return [
    node.missing === true ? t("explorer.missing") : "",
    node.loads > 1 ? t("explorer.readTimes", { count: node.loads }) : "",
    node.editable ? "" : t("explorer.readOnly"),
  ].filter(Boolean).join(" · ");
}

// ConfigFileList は、Include をたどって読み込まれる設定ファイルを、読み込んだ Include の下に字下げして並べる。
// ~/.ssh の外のファイルは開けないので、パスと理由だけを出す。
// The tree already shows which folder a file is in, so a row names only the file; its path is the button's name
// and tooltip. In a narrow column the whole path was cut off before the file's name.
function fileName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

// breakablePath lets an Include pattern wrap after a slash rather than in the middle of a folder's name.
function breakablePath(pattern: string): ReactNode {
  const parts = pattern.split("/");
  return parts.map((part, index) => (
    <Fragment key={index}>{part}{index < parts.length - 1 ? <>/<wbr /></> : null}</Fragment>
  ));
}

export function ConfigFileList({
  files,
  openPath,
  onOpen,
}: {
  files: FileNode[];
  openPath: string;
  onOpen: (path: string) => void;
}) {
  const t = useTranslate();
  return (
    <ul className="flex flex-col gap-0.5">
      {configTreeRows(files).map((row) => {
        if (row.kind === "include") {
          const { include } = row;
          return (
            <li
              key={`${row.owner.file.absolute}:${include.line}:${include.pattern}`}
              style={{ marginLeft: row.depth * depthIndentPx + includeIndentPx }}
              className="px-2.5 pb-1.5 pt-0.5"
            >
              <div className="min-w-0 overflow-hidden text-xs">
                <span className="block break-words font-mono text-ink-faint">{breakablePath(include.pattern)}</span>
                {include.condition === undefined ? null : (
                  <span className="mt-0.5 block break-words text-notice-ink">
                    {t("explorer.insideCondition", { condition: include.condition })}
                  </span>
                )}
              </div>
            </li>
          );
        }
        const { node } = row;
        const indent = { marginLeft: row.depth * depthIndentPx };
        if (node.file.path === undefined) {
          return (
            <li key={node.file.absolute} style={indent} className="flex items-start gap-2 px-2.5 py-2">
              <Icon name="config" className="mt-px size-[15px] shrink-0 text-ink-faint" />
              <p className="min-w-0 text-xs">
                <span className="block break-all font-mono text-ink-muted">{node.file.absolute}</span>
                <span className="mt-1 block leading-5 text-notice-ink">{t("explorer.externalFile")}</span>
              </p>
            </li>
          );
        }
        const path = node.file.path;
        const current = path === openPath;
        const notes = fileNotes(t, node);
        return (
          <li
            key={node.file.absolute}
            style={indent}
            className={`rounded-lg ${current ? "bg-select-fill" : "transition-colors duration-150 hover:bg-hover"}`}
          >
            <button
              type="button"
              aria-current={current ? "true" : "false"}
              aria-label={path}
              title={path}
              onClick={() => onOpen(path)}
              className="flex min-h-10 w-full min-w-0 items-center gap-2 px-2.5 py-2 text-left md:min-h-0"
            >
              <span data-config-node-icon aria-hidden="true" className={`shrink-0 ${current ? "text-ink" : "text-ink-muted"}`}>
                <Icon name="config" className="size-[15px]" />
              </span>
              <span className={`min-w-0 truncate font-mono text-[13px] text-ink ${current ? "font-semibold" : ""}`}>{fileName(path)}</span>
            </button>
            {notes === "" ? null : <p className="-mt-1 pb-2 pl-[35px] pr-2.5 text-xs text-ink-faint">{notes}</p>}
          </li>
        );
      })}
    </ul>
  );
}
