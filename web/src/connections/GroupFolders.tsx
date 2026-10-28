import type { ReactNode } from "react";
import type { GroupView, HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { KeysApi } from "../keys/api";
import { sectionHeading } from "../ui/form";
import { Icon, type IconName } from "../ui/icons";
import { connectionFilesOf, keyFilesOf } from "./groupFolderContents";
import { useKeyItems } from "./useKeyItems";

type GroupFoldersProps = {
  group: GroupView;
  hosts: readonly HostEntry[];
  keys?: Pick<KeysApi, "inventory">;
};

// GroupFolders は、グループの編集欄の「フォルダ」の節である。グループの2つのフォルダの中身と、
// ~/.ssh/config でそのフォルダを読み込む行を見せる。グループは設定の上の名前ではなく、ディスクの上の
// フォルダなので、何がどこにあるかをここで分かるようにする。
export function GroupFolders({ group, hosts, keys }: GroupFoldersProps) {
  const t = useTranslate();
  const keyItems = useKeyItems(keys);
  const files = connectionFilesOf(hosts, group.name);
  const keyFiles = keyItems.kind === "ready" ? keyFilesOf(keyItems.items, group.name) : [];

  return (
    <section aria-labelledby="group-folders-heading" className="flex flex-col gap-3 border-t border-line pt-5">
      <h4 id="group-folders-heading" className={sectionHeading}>{t("groupFolders.heading")}</h4>
      <div className="grid gap-4 sm:grid-cols-2">
        <Folder icon="groups" path={`${group.directory}/`}>
          {!group.directoryPresent ? (
            <p className="text-xs text-notice-ink">{t("groupFolders.missing")}</p>
          ) : files.length === 0 ? (
            <p className="text-xs text-ink-faint">{t("groupFolders.noConnections")}</p>
          ) : files.map((file) => (
            <div key={file.name} className="min-w-0">
              <p className="truncate font-mono text-xs text-ink-muted">{file.name}</p>
              <p className="break-words text-xs text-ink-faint">{file.aliases.join(", ")}</p>
            </div>
          ))}
        </Folder>
        <Folder icon="keys" path={`${group.keyDirectory}/`}>
          {keyItems.kind === "loading" ? (
            <p className="text-xs text-ink-faint">{t("groupFolders.keysLoading")}</p>
          ) : keyItems.kind === "failed" ? (
            <p className="text-xs text-notice-ink">{t("groupFolders.keysFailed")}</p>
          ) : keyFiles.length === 0 ? (
            <p className="text-xs text-ink-faint">{t("groupFolders.noKeys")}</p>
          ) : keyFiles.map((file) => (
            <p key={file.name} className="flex min-w-0 items-baseline gap-1.5">
              <span className="min-w-0 truncate font-mono text-xs text-ink-muted">{file.name}</span>
              {file.companions.length === 0 ? null : (
                <span className="shrink-0 font-mono text-[11.5px] text-ink-faint">{file.companions.join(" ")}</span>
              )}
            </p>
          ))}
        </Folder>
      </div>
      <p className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 text-xs text-ink-faint">
        {t("groupFolders.includeLine")}
        <code className="rounded bg-surface-subtle px-1.5 py-0.5 font-mono text-[11.5px] text-ink-muted">{`Include ${group.directory}/*.conf`}</code>
      </p>
    </section>
  );
}

function Folder({ icon, path, children }: { icon: IconName; path: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="flex items-center gap-1.5 font-mono text-xs text-ink">
        <Icon name={icon} className="size-3.5 shrink-0 text-ink-faint" />
        <span className="truncate">{path}</span>
      </p>
      <div className="mt-1.5 flex flex-col gap-1 pl-5">{children}</div>
    </div>
  );
}
