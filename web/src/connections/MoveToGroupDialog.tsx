import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { Spinner } from "../ui/Spinner";
import { Button } from "../ui/surface";
import { usePendingResult } from "../ui/usePendingResult";
import type { GroupListing } from "./groupListing";

type MoveToGroupDialogProps = {
  // 動かす接続先の名前。1件なら見出しに名前を出す。
  aliases: readonly string[];
  groups: readonly GroupListing[];
  // 選んだ接続先がすべているグループ。そこへは移せないので選べなくする。
  currentGroup: string | null;
  // Promise を返すと、移し終えるまで開いたまま待ち、選んだ行に回転の印を出す。
  onPick: (group: string) => void | Promise<unknown>;
  onCancel: () => void;
  // 移せなかった理由。背面に出すとダイアログに隠れるので、中に出す。
  error?: string;
};

const errorId = "move-to-group-error";

// MoveToGroupDialog は、選んだ接続先の移動先を、グループの一覧から1回のタップで選ばせる。
// スマホでは下から出る。
export function MoveToGroupDialog({ aliases, groups, currentGroup, onPick, onCancel, error = "" }: MoveToGroupDialogProps) {
  const t = useTranslate();
  const move = usePendingResult();
  const [picked, setPicked] = useState<string | null>(null);
  const rows = [
    ...groups.map((group) => ({ name: group.name, label: group.label, depth: group.depth })),
    { name: "", label: t("tree.ungrouped"), depth: 0 },
  ];
  return (
    <ModalShell
      labelledBy="move-to-group-heading"
      {...(error === "" ? {} : { describedBy: errorId })}
      onDismiss={onCancel}
      dismissible={!move.pending}
      closeOnOutside
      placement="sheet"
      panelClassName="flex max-h-[80dvh] w-full max-w-md flex-col rounded-lg"
    >
      <h2 id="move-to-group-heading" className="border-b border-line px-4 py-3 text-base font-semibold text-ink">
        {aliases.length === 1 ? t("moveDialog.headingOne", { alias: aliases[0] ?? "" }) : t("moveDialog.heading", { count: aliases.length })}
      </h2>
      <ul className="min-h-0 flex-1 overflow-y-auto p-2">
        {rows.map((row) => {
          const moving = move.pending && picked === row.name;
          return (
            <li key={row.name || "ungrouped"}>
              <button
                type="button"
                disabled={row.name === currentGroup || move.pending}
                aria-busy={moving || undefined}
                onClick={() => {
                  setPicked(row.name);
                  move.watch(onPick(row.name));
                }}
                style={{ paddingInlineStart: `${0.75 + row.depth * 1.25}rem` }}
                className="flex min-h-11 w-full items-center gap-2 rounded-md pr-3 text-left text-sm text-ink transition-colors duration-150 hover:bg-hover md:min-h-9"
              >
                <Icon name="groups" className="size-4 text-ink-faint" />
                <span className="min-w-0 flex-1 truncate">{row.label}</span>
                {moving ? <Spinner /> : null}
              </button>
            </li>
          );
        })}
      </ul>
      {error === "" ? null : <p id={errorId} role="alert" className="px-4 py-2 text-sm text-danger">{error}</p>}
      <div className="flex justify-end border-t border-line px-4 py-3">
        <Button disabled={move.pending} onClick={onCancel}>{t("groupDialog.cancel")}</Button>
      </div>
    </ModalShell>
  );
}
