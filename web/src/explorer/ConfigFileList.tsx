import type { FileNode } from "../api/config";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";

// ConfigFileList は、Include をたどって読み込まれる設定ファイルの一覧である。
// ~/.ssh の外のファイルは開けないので、パスと理由だけを出す。
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
    <ul className="flex max-h-96 flex-col gap-0.5 overflow-y-auto p-2 lg:max-h-none lg:flex-1">
      {files.map((node) => {
        const current = (node.file.path ?? node.file.absolute) === openPath;
        const state = t("explorer.fileState", {
          missing: node.missing === true ? t("explorer.missing") : "",
          loads: node.loads > 1 ? t("explorer.readTimes", { count: node.loads }) : "",
          editable: node.editable ? t("explorer.editable") : t("explorer.readOnly"),
        });
        return (
          <li key={node.file.absolute} className={`rounded-lg ${current ? "bg-select-fill" : "hover:bg-surface-subtle"}`}>
            <div className="flex items-start gap-2.5 px-2.5 py-2">
              <span
                data-config-node-icon
                aria-hidden="true"
                className={`mt-5 shrink-0 md:mt-2.5 [@media(pointer:coarse)]:mt-[1.375rem] ${current ? "text-accent" : "text-ink-faint"}`}
              >
                <Icon name="config" className="h-4 w-4" />
              </span>
              <div className="min-w-0 flex-1">
                {node.file.path === undefined ? (
                  <p className="text-sm text-ink-muted">
                    <span className="block break-all font-mono text-xs">{node.file.absolute}</span>
                    <span className="mt-1 block text-xs leading-5 text-notice-ink">{t("explorer.externalFile")}</span>
                  </p>
                ) : (
                  <button
                    type="button"
                    aria-current={current ? "true" : "false"}
                    onClick={() => onOpen(node.file.path ?? "")}
                    className={`block min-h-10 w-full truncate text-left font-mono text-sm md:min-h-0 ${current ? "font-semibold text-ink" : "text-ink-muted hover:text-ink"}`}
                  >
                    {node.file.path}
                  </button>
                )}
                <p className="mt-0.5 text-xs text-ink-faint">{state}</p>
                {(node.includes ?? []).map((include) => (
                  <div
                    key={`${node.file.absolute}:${include.line}:${include.pattern}`}
                    className="mt-2 min-w-0 overflow-hidden rounded-md bg-surface-subtle px-2 py-1.5 text-xs text-ink-muted"
                  >
                    <span className="block break-all font-mono leading-5">{include.pattern}</span>
                    {include.condition === undefined ? null : (
                      <span className="mt-0.5 block break-words leading-5 text-notice-ink">
                        {t("explorer.insideCondition", { condition: include.condition })}
                      </span>
                    )}
                    <ul className="mt-1">
                      {(include.matches ?? []).map((match) => (
                        <li
                          key={match.absolute}
                          className="flex min-w-0 items-center gap-1 font-mono text-ink-faint"
                          title={match.path ?? match.absolute}
                        >
                          <Icon name="arrowRight" className="size-3" />
                          <span className="truncate">{match.path ?? match.absolute}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}
