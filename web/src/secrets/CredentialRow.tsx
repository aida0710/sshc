import type { ReactNode } from "react";
import type { CredentialsApi } from "../api/credentials";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { InlinePanel } from "../ui/InlinePanel";
import { Button } from "../ui/surface";
import { rowAssignmentText, rowHosts, type CredentialRow as Row } from "./credentialRows";
import { TOTPCurrentCode, TOTPNeighbourCodes } from "./TOTPCode";
import { useTOTPCodes } from "./useTOTPCodes";

// What a row can open under itself. Only one row has one open at a time; the add form above the list is
// apart from them, so neither closes the other and loses what was typed.
export type RowPanel = "codes" | "hosts" | "edit";

export type RowActions = {
  onOpenPanel: (panel: RowPanel | null) => void;
  onDelete: () => void;
  onUnassignTOTP: (host: string) => void;
};

// The columns of a row. On a phone the assignment line sits under the name; where the board is wide it
// takes a column of its own, under the table's header (see SecretsPanel), which is a grid of its own and
// lines up only because no column takes its width from what is in it. The code column holds a six-digit
// code at 22px with its 26px ring.
export const totpColumns = "grid-cols-[minmax(0,1fr)_9.5rem_2.25rem] @2xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)_9.5rem_2.25rem]";
export const plainColumns = "grid-cols-[minmax(0,1fr)_2.25rem] @2xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)_2.25rem]";

// CredentialRow is one line of a Vault page: the name and where it is used, a one-time password's current
// code, and the row's menu. What the menu opens (the codes around the current one, the hosts it is
// assigned to, the edit form) shows under the row.
export function CredentialRow({ row, keyHostUsageComplete, panel, editPanel, api, actions }: {
  row: Row;
  keyHostUsageComplete: boolean;
  panel: RowPanel | null;
  editPanel: ReactNode;
  api: Pick<CredentialsApi, "totpCodes">;
  actions: RowActions;
}) {
  const isTOTP = row.source === "named" && row.credential.kind === "totp";
  return isTOTP
    ? <TOTPRow row={row} keyHostUsageComplete={keyHostUsageComplete} panel={panel} editPanel={editPanel} api={api} actions={actions} />
    : <RowLayout row={row} keyHostUsageComplete={keyHostUsageComplete} panel={panel} editPanel={editPanel} actions={actions} code={null} neighbours={null} />;
}

function TOTPRow({ row, keyHostUsageComplete, panel, editPanel, api, actions }: Parameters<typeof CredentialRow>[0]) {
  const codes = useTOTPCodes(row.name, api);
  return (
    <RowLayout
      row={row}
      keyHostUsageComplete={keyHostUsageComplete}
      panel={panel}
      editPanel={editPanel}
      actions={actions}
      code={<TOTPCurrentCode name={row.name} codes={codes} />}
      neighbours={codes.state === "ready" ? <TOTPNeighbourCodes name={row.name} codes={codes.codes} /> : null}
    />
  );
}

function RowLayout({ row, keyHostUsageComplete, panel, editPanel, actions, code, neighbours }: {
  row: Row;
  keyHostUsageComplete: boolean;
  panel: RowPanel | null;
  editPanel: ReactNode;
  actions: RowActions;
  code: ReactNode;
  neighbours: ReactNode;
}) {
  const t = useTranslate();
  const assignment = rowAssignmentText(row, keyHostUsageComplete, t);
  const hosts = rowHosts(row);
  const menu = rowMenu({ row, hosts, panel, hasCode: code !== null, neighboursReady: neighbours !== null, actions, t });
  const open = panel === "codes" ? neighbours : panel === "hosts" ? <AssignedHosts hosts={hosts} onUnassign={actions.onUnassignTOTP} /> : panel === "edit" ? editPanel : null;
  return (
    <li className="border-t border-line first:border-t-0">
      <article aria-label={row.label} className={`grid min-h-16 items-center gap-x-3 py-2.5 ${code === null ? plainColumns : totpColumns}`}>
        <div className="min-w-0">
          <h4 className="font-mono text-[13.5px] font-semibold leading-[19px] text-ink [overflow-wrap:anywhere]">{row.name}</h4>
          <p className="mt-0.5 text-xs leading-[17px] text-ink-faint [overflow-wrap:anywhere] @2xl:hidden">{assignment}</p>
        </div>
        <p className="hidden min-w-0 font-mono text-xs text-ink-muted [overflow-wrap:anywhere] @2xl:block">{assignment}</p>
        {code}
        <ActionMenu
          label={t("secrets.actions", { name: row.label })}
          items={menu}
          triggerClassName="flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill hover:text-ink"
          iconClassName="size-5"
        />
      </article>
      {open === null ? null : <InlinePanel className="pb-3">{open}</InlinePanel>}
    </li>
  );
}

function rowMenu({ row, hosts, panel, hasCode, neighboursReady, actions, t }: {
  row: Row;
  hosts: string[];
  panel: RowPanel | null;
  hasCode: boolean;
  // The codes around the current one come with it; until they have, there is nothing to open.
  neighboursReady: boolean;
  actions: RowActions;
  t: ReturnType<typeof useTranslate>;
}): ActionMenuItem[] {
  const toggle = (next: RowPanel) => () => actions.onOpenPanel(panel === next ? null : next);
  const items: ActionMenuItem[] = [];
  if (hasCode) {
    items.push({
      label: t(panel === "codes" ? "secrets.hideNeighbourCodes" : "secrets.showNeighbourCodes"),
      onSelect: toggle("codes"),
      disabled: !neighboursReady,
    });
    items.push({ label: t("secrets.assignedHostsCount", { count: hosts.length }), onSelect: toggle("hosts") });
  }
  if (row.source === "named") items.push({ label: t("secrets.editShort"), onSelect: toggle("edit") });
  items.push({ label: t("secrets.deleteShort"), tone: "danger", onSelect: actions.onDelete });
  return items;
}

// AssignedHosts lists the hosts a one-time password is assigned to, each with a way to take it off. The
// assignment itself is made in Connections.
function AssignedHosts({ hosts, onUnassign }: { hosts: string[]; onUnassign: (host: string) => void }) {
  const t = useTranslate();
  return (
    <div className="rounded-lg bg-well px-3 py-2">
      <p className="mb-1.5 text-xs text-ink-faint">{t("secrets.assignedHosts")}</p>
      {hosts.length === 0 ? (
        <p className="text-sm text-ink-muted">{t("secrets.noAssignedHosts")}</p>
      ) : (
        <ul aria-label={t("secrets.assignedHosts")} className="flex flex-col">
          {hosts.map((host) => (
            <li key={host} className="flex min-h-9 items-center gap-2 border-t border-line first:border-t-0">
              <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-ink">{host}</span>
              <Button small aria-label={t("secrets.unassignTOTP", { host })} onClick={() => onUnassign(host)}>
                {t("secrets.unassign")}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
