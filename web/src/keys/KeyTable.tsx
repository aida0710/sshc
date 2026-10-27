import { Fragment, useEffect, useRef, type DragEvent, type ReactNode } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import type { KeyCertificate, KeyInventoryResponse, KeyItem } from "./api";
import { ActionMenu, revealOnHover, type ActionMenuItem } from "../ui/ActionMenu";
import { tableHeadCell, tableHeadRow } from "../ui/form";
import { Icon } from "../ui/icons";
import { describeAlgorithm, describeKeyKind, noteLabels } from "./labels";
import { groupOfKeyPath, keyItemGroups, keyListPath } from "./organizer";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";

type KeySort = "file" | "kind" | "state";

export type KeyRowActions = {
  onSelect: (item: KeyItem) => void;
  onToggleChosen: (item: KeyItem, chosen: boolean) => void;
  onBeginDrag: (event: DragEvent<HTMLElement>, item: KeyItem) => void;
  onEndDrag: () => void;
  onReveal: (item: KeyItem) => void;
  // onShowPublicKey reads a public key or certificate file: the row's own, or the one paired with a
  // private key.
  onShowPublicKey: (item: KeyItem) => void;
  onManageStoredPassphrase: (item: KeyItem) => void;
  onAddToAgent: (item: KeyItem) => void;
  onRemoveFromAgent: (item: KeyItem) => void;
  onChangePassphrase: (item: KeyItem) => void;
  onRelocate: (item: KeyItem) => void;
  onMoveToTrash: (item: KeyItem) => void;
};

// The table is a table where its container (keytable, around it in KeysScreen) is at least 36rem wide, and
// one card per key where it is narrower (a phone, or a page squeezed by the details pane), so no column
// has to be scrolled to. The table lays out its columns at fixed widths, so a long file name wraps in its
// own column instead of pushing the last one out. The classes spell out @xl/keytable each time so
// Tailwind can find them.

// A row's state, as the small round tags of the design: green for in use, amber for something to look at.
const tag = "inline-flex h-5 items-center gap-1 whitespace-nowrap rounded-full px-[7px] text-[11.5px]";
const neutralTag = `${tag} bg-well text-ink-muted`;
const goodTag = `${tag} bg-live/15 text-live`;
const attentionTag = `${tag} bg-notice text-notice-ink`;

export function KeyTable({
  items,
  inventory,
  chosen,
  selected,
  now,
  revealRelated = false,
  inlinePanel,
  actions,
}: {
  items: KeyItem[];
  inventory: KeyInventoryResponse;
  chosen: ReadonlySet<string>;
  selected: string | null;
  now: number;
  // revealRelated lists a key's public files and certificates as rows of their own (while searching, so
  // a match on one of them shows).
  revealRelated?: boolean;
  // inlinePanel opens under one row: a form or a key's text, for the action chosen from that row.
  inlinePanel: { itemId: string; content: ReactNode } | null;
  actions: KeyRowActions;
}) {
  const t = useTranslate();
  const keySort = useTableSort<KeySort>("file");
  // 種類と状態のセルは1つの文ではなく、ラベルやバッジを並べて出す。並べ替えは、
  // それらの元の値をつないだ文字列で比べる。
  const keySortValue = (item: KeyItem, column: KeySort): SortValue => {
    switch (column) {
      case "file": return item.relativePath;
      case "kind": return `${item.kind}\u0000${item.algorithm}\u0000${item.bits}`;
      case "state": return keyState(item, inventory);
    }
  };
  const sortedGroups = keySort.sorted(keyItemGroups(items), (group, column) => keySortValue(group.primary, column));
  const displayed = sortedGroups.flatMap((group) => [
    { item: group.primary, related: group.related, relatedTo: null as string | null },
    ...(revealRelated
      ? keySort.sorted(group.related, keySortValue).map((item) => ({ item, related: [] as KeyItem[], relatedTo: group.primary.id }))
      : []),
  ]);

  return (
    <table className="block w-full text-left text-[13px] @xl/keytable:table @xl/keytable:table-fixed">
      <caption className="sr-only">{t("keys.tableCaption")}</caption>
      <thead className={`hidden @xl/keytable:table-header-group`}>
        <tr className={tableHeadRow}>
          <th scope="col" className={`${tableHeadCell} w-9`}>
            <span className="sr-only">{t("keys.colChoose")}</span>
          </th>
          <SortableTableHeader column="file" {...keySort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFile")}</SortableTableHeader>
          <SortableTableHeader column="kind" {...keySort.headerProps} className={`${tableHeadCell} w-[7.5rem] whitespace-nowrap`}>{t("keys.colKind")}</SortableTableHeader>
          <SortableTableHeader column="state" {...keySort.headerProps} className={`${tableHeadCell} w-[11.5rem] whitespace-nowrap`}>{t("keys.colState")}</SortableTableHeader>
          <th scope="col" className={`${tableHeadCell} w-11`}>
            <span className="sr-only">{t("keys.colActions")}</span>
          </th>
        </tr>
      </thead>
      <tbody className={`block @xl/keytable:table-row-group`}>
        {displayed.map(({ item, related, relatedTo }) => {
          const isSelected = selected === item.id;
          const movable = renameable(item, inventory.items);
          const menu = rowActionItems({ item, related, inventory, actions, t });
          return (
            <Fragment key={item.id}>
              <tr
                data-key-related-to={relatedTo ?? undefined}
                draggable={movable}
                onDragStart={movable ? (event) => actions.onBeginDrag(event, item) : undefined}
                onDragEnd={movable ? actions.onEndDrag : undefined}
                className={`group/row grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-2 border-b border-line py-2 pl-1 last:border-b-0 @xl/keytable:table-row @xl/keytable:border-b-0 ${
                  isSelected ? "bg-select-fill" : "hover:bg-hover"
                } ${relatedTo === null ? "" : "bg-surface-subtle"}`}
              >
                <td className={`hidden border-line align-middle @xl/keytable:table-cell @xl/keytable:border-t @xl/keytable:py-[9px] @xl/keytable:pl-2.5`}>
                  {movable ? (
                    <label className="grid place-items-center">
                      <input
                        type="checkbox"
                        aria-label={t("keys.chooseKey", { path: item.relativePath })}
                        checked={chosen.has(item.id)}
                        onChange={(event) => actions.onToggleChosen(item, event.target.checked)}
                        className="size-4 accent-accent"
                      />
                    </label>
                  ) : null}
                </td>
                <td className={`relative min-w-0 border-line align-middle @xl/keytable:table-cell @xl/keytable:border-t @xl/keytable:px-2.5 @xl/keytable:py-[9px] ${relatedTo === null ? "" : "pl-4 @xl/keytable:pl-6"}`}>
                  {/* The name's button reaches over the whole cell (its ::before), so the line under the name
                      is part of what a finger can tap without making the row taller. */}
                  <button
                    type="button"
                    aria-pressed={isSelected}
                    onClick={() => actions.onSelect(item)}
                    className="block min-h-0! max-w-full text-left text-[13.5px] text-ink [overflow-wrap:anywhere] before:absolute before:inset-0 before:content-[''] hover:underline"
                  >
                    {keyListPath(item.relativePath)}
                  </button>
                  <span className={`block font-mono text-[11.5px] text-ink-faint @xl/keytable:hidden`}>{kindText(item, t)}</span>
                  <span className={`hidden truncate text-[11.5px] text-ink-faint @xl/keytable:block`}>{whereText(item, t)}</span>
                </td>
                <td className={`hidden whitespace-nowrap border-line align-middle font-mono text-xs text-ink-muted @xl/keytable:table-cell @xl/keytable:border-t @xl/keytable:px-2.5 @xl/keytable:py-[9px]`}>
                  {kindText(item, t)}
                  {item.certificate === undefined ? null : (
                    <span className="block font-sans text-[11.5px]">{certificateValidity(item.certificate, now, t)}</span>
                  )}
                </td>
                <td className={`border-line align-middle @xl/keytable:table-cell @xl/keytable:border-t @xl/keytable:px-2.5 @xl/keytable:py-[9px]`}>
                  <span className="@xl/keytable:hidden"><KeySummaryTag item={item} t={t} /></span>
                  <span className={`hidden flex-wrap gap-1.5 @xl/keytable:flex`}><KeyStateTags item={item} inventory={inventory} t={t} /></span>
                </td>
                <td className={`border-line pr-1 text-right align-middle @xl/keytable:table-cell @xl/keytable:border-t @xl/keytable:py-[9px] @xl/keytable:pr-2.5`}>
                  {menu.length === 0 ? null : (
                    <ActionMenu
                      label={t("keys.rowActions", { path: item.relativePath })}
                      items={menu}
                      triggerClassName={`ml-auto flex size-10 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink @xl/keytable:size-7 ${isSelected ? "" : revealOnHover}`}
                    />
                  )}
                </td>
              </tr>
              {inlinePanel?.itemId === item.id ? (
                <tr data-key-panel-for={item.id} className={`block @xl/keytable:table-row`}>
                  <td colSpan={5} className={`block px-1 pb-3 pt-1 @xl/keytable:table-cell @xl/keytable:px-2.5`}>
                    <InlinePanel>{inlinePanel.content}</InlinePanel>
                  </td>
                </tr>
              ) : null}
            </Fragment>
          );
        })}
        {displayed.length === 0 && (
          <tr className={`block @xl/keytable:table-row`}>
            <td colSpan={5} className={`block p-8 text-center text-sm text-ink-muted @xl/keytable:table-cell`}>
              {inventory.items.length === 0 ? t("keys.inventoryEmpty") : t("keys.noMatches")}
            </td>
          </tr>
        )}
      </tbody>
    </table>
  );
}

// InlinePanel brings the form it opens into view, since the row it belongs to may sit at the bottom of
// the window.
function InlinePanel({ children }: { children: ReactNode }) {
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    panel.current?.scrollIntoView?.({ block: "nearest" });
  }, []);
  return <div ref={panel}>{children}</div>;
}

function rowActionItems({ item, related, inventory, actions, t }: {
  item: KeyItem;
  related: KeyItem[];
  inventory: KeyInventoryResponse;
  actions: KeyRowActions;
  t: Translate;
}): ActionMenuItem[] {
  const menu: ActionMenuItem[] = [];
  if (item.kind === "public_key" || item.kind === "certificate") {
    menu.push({ label: t("keys.showPublicKey"), onSelect: () => actions.onShowPublicKey(item) });
  }
  if (item.kind === "private_key") {
    const pairedPublic = related.find((candidate) => candidate.kind === "public_key");
    const certificate = related.find((candidate) => candidate.kind === "certificate");
    const heldByAgent = agentHolds(inventory, item);
    menu.push({ label: t("keys.showPrivateKey"), onSelect: () => actions.onReveal(item) });
    if (pairedPublic !== undefined) menu.push({ label: t("keys.showPublicKey"), onSelect: () => actions.onShowPublicKey(pairedPublic) });
    if (certificate !== undefined) menu.push({ label: t("keys.showCertificate"), onSelect: () => actions.onShowPublicKey(certificate) });
    menu.push({
      label: t(heldByAgent ? "keys.removeFromAgent" : "keys.addToAgent"),
      disabled: !inventory.agentAvailable,
      onSelect: () => heldByAgent ? actions.onRemoveFromAgent(item) : actions.onAddToAgent(item),
    });
    if (item.encrypted) menu.push({ label: t("keys.manageStoredPassphrase"), onSelect: () => actions.onManageStoredPassphrase(item) });
    menu.push({ label: t("keys.changePassphrase"), onSelect: () => actions.onChangePassphrase(item) });
  }
  if (renameable(item, inventory.items)) menu.push({ label: t("keys.relocate"), onSelect: () => actions.onRelocate(item) });
  if (item.kind === "private_key") menu.push({ label: t("keys.moveToTrash"), tone: "danger", onSelect: () => actions.onMoveToTrash(item) });
  return menu;
}

// kindText is the 種別 column: the algorithm of a private key, and what a file is with its algorithm for
// anything else (a public key or certificate on its own, or a file that is not a key).
function kindText(item: KeyItem, t: Translate): string {
  const algorithm = describeAlgorithm(item.algorithm, item.bits);
  if (item.kind === "private_key" && algorithm !== "") return algorithm;
  const kind = describeKeyKind(item.kind, t);
  return algorithm === "" ? kind : `${kind} · ${algorithm}`;
}

// whereText is the line under a file's name: the group it is in, or where it is under ~/.ssh.
function whereText(item: KeyItem, t: Translate): string {
  const group = groupOfKeyPath(item.relativePath);
  return group === "" ? `~/.ssh/${item.relativePath}` : t("keys.inGroup", { group });
}

function KeyStateTags({ item, inventory, t }: { item: KeyItem; inventory: KeyInventoryResponse; t: Translate }) {
  return (
    <>
      {item.permissionRisk ? (
        <span className={attentionTag}><Icon name="warning" className="size-3" />{t("keys.permissionRisk")}</span>
      ) : null}
      {item.notes.map((note) => (
        <span key={note} className={attentionTag}>{note in noteLabels ? t(noteLabels[note]!) : note}</span>
      ))}
      {agentHolds(inventory, item) ? (
        <span className={goodTag}><span aria-hidden="true" className="size-1.5 rounded-full bg-live" />{t("keys.stateInAgent")}</span>
      ) : null}
      {item.references.length > 0 ? (
        <span className={goodTag}><Icon name="check" className="size-3" />{t("keys.stateUsedBy", { count: item.references.length })}</span>
      ) : item.kind === "private_key" ? (
        <span className={neutralTag}>{t("keys.stateUnused")}</span>
      ) : null}
    </>
  );
}

// KeySummaryTag is the one tag a card shows where there is no room for all of them: what needs a look
// first, then whether the key is used.
function KeySummaryTag({ item, t }: { item: KeyItem; t: Translate }) {
  if (item.permissionRisk || item.notes.length > 0) return <span className={attentionTag}>{t("keys.stateAttention")}</span>;
  if (item.references.length > 0) return <span className={goodTag}>{t("keys.stateInUse")}</span>;
  if (item.kind === "private_key") return <span className={neutralTag}>{t("keys.stateUnused")}</span>;
  return null;
}

function certificateValidity(certificate: KeyCertificate, now: number, t: Translate): string {
  if (certificate.neverExpires) return t("keys.certNeverExpires");
  const when = certificateExpiry(certificate);
  return certificate.validBefore * 1000 <= now ? t("keys.certExpired", { when }) : t("keys.certValidUntil", { when });
}

function certificateExpiry(certificate: KeyCertificate): string {
  return `${new Date(certificate.validBefore * 1000).toISOString().slice(0, 16).replace("T", " ")}Z`;
}

export function certificateLines(
  certificate: KeyCertificate,
  now: number,
  t: Translate,
): { text: string; expired: boolean }[] {
  const lines: { text: string; expired: boolean }[] = [];
  if (certificate.keyId !== "") lines.push({ text: t("keys.certKeyId", { keyId: certificate.keyId }), expired: false });
  if (certificate.principals.length > 0) {
    lines.push({ text: t("keys.certFor", { principals: certificate.principals.join(", ") }), expired: false });
  } else {
    lines.push({ text: t("keys.certAnyPrincipal"), expired: false });
  }
  if (certificate.neverExpires) {
    lines.push({ text: t("keys.certNeverExpires"), expired: false });
  } else {
    const expired = certificate.validBefore * 1000 <= now;
    lines.push({ text: certificateValidity(certificate, now, t), expired });
  }
  if (certificate.signedKeyType !== "") {
    lines.push({
      text: t("keys.certSigns", {
        keyType: certificate.signedKeyType,
        fingerprint: certificate.signedKeyFingerprint,
      }).trim(),
      expired: false,
    });
  }
  return lines;
}

export function renameable(item: KeyItem, items: KeyItem[]): boolean {
  if (item.kind === "private_key") return true;
  if (item.kind !== "public_key" && item.kind !== "certificate") return false;
  const fingerprint =
    item.kind === "certificate" && item.certificate !== undefined
      ? item.certificate.signedKeyFingerprint
      : item.fingerprint;
  if (fingerprint === "") return true;
  return !items.some((candidate) => candidate.kind === "private_key" && candidate.fingerprint === fingerprint);
}

export function agentHolds(inventory: KeyInventoryResponse, item: KeyItem): boolean {
  if (!inventory.agentAvailable || item.fingerprint === "") return false;
  return inventory.agentIdentities.some((identity) => identity.fingerprint === item.fingerprint);
}

function keyState(item: KeyItem, inventory: KeyInventoryResponse): string {
  return [
    item.permissionRisk ? "permission-risk" : "",
    agentHolds(inventory, item) ? "in-agent" : "",
    item.references.length > 0 ? `used-${item.references.length}` : "",
    ...item.notes,
  ].join("\u0000");
}
