import { Fragment, useState, type DragEvent, type ReactNode } from "react";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { useTranslate, type Translate } from "../i18n/context";
import type { KeyCertificate, KeyInventoryResponse, KeyItem } from "./api";
import { tableHeadCell, tableHeadRow } from "../ui/form";
import { describeKeyKind, noteLabels, rowAction, rowDanger, rowPrimary } from "./labels";
import { keyItemGroups } from "./organizer";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";

type KeySort = "file" | "kind" | "state";

export type KeyRowActions = {
  onSelect: (item: KeyItem) => void;
  onToggleChosen: (item: KeyItem, chosen: boolean) => void;
  onBeginDrag: (event: DragEvent<HTMLSpanElement>, item: KeyItem) => void;
  onEndDrag: () => void;
  onReveal: (item: KeyItem) => void;
  onShowPublicKey: (item: KeyItem) => void;
  onManageStoredPassphrase: (item: KeyItem) => void;
  onAddToAgent: (item: KeyItem) => void;
  onRemoveFromAgent: (item: KeyItem) => void;
  onToggleDetails: (item: KeyItem) => void;
  onChangePassphrase: (item: KeyItem) => void;
  onRelocate: (item: KeyItem) => void;
  onMoveToTrash: (item: KeyItem) => void;
};

export function KeyTable({
  items,
  inventory,
  chosen,
  selected,
  detailsFor,
  now,
  revealRelated = false,
  inlinePanel,
  actions,
}: {
  items: KeyItem[];
  inventory: KeyInventoryResponse;
  chosen: ReadonlySet<string>;
  selected: string | null;
  detailsFor: string;
  now: number;
  revealRelated?: boolean;
  inlinePanel: { itemId: string; content: ReactNode } | null;
  actions: KeyRowActions;
}) {
  const t = useTranslate();
  const [expandedRelated, setExpandedRelated] = useState<ReadonlySet<string>>(new Set());
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
  const displayed = sortedGroups.flatMap((group) => {
    const relatedExpanded = revealRelated || expandedRelated.has(group.primary.id);
    return [
      {
        item: group.primary,
        relatedCount: group.related.length,
        relatedExpanded,
        relatedTo: null as string | null,
      },
      ...(relatedExpanded
        ? keySort.sorted(group.related, keySortValue).map((item) => ({
            item,
            relatedCount: 0,
            relatedExpanded: false,
            relatedTo: group.primary.id,
          }))
        : []),
    ];
  });

  function toggleRelated(item: KeyItem) {
    setExpandedRelated((current) => {
      const next = new Set(current);
      if (next.has(item.id)) next.delete(item.id);
      else next.add(item.id);
      return next;
    });
  }

  return (
    <table className="block w-full text-left text-sm md:table md:min-w-[56rem]">
      <caption className="sr-only">{t("keys.tableCaption")}</caption>
      <thead className="hidden md:table-header-group">
        <tr className={`${tableHeadRow} bg-surface-subtle`}>
          <th scope="col" className={`${tableHeadCell} w-12 pl-3`}>
            <span className="sr-only">{t("keys.colChoose")}</span>
          </th>
          <SortableTableHeader column="file" {...keySort.headerProps} className={`${tableHeadCell} w-[30%] whitespace-nowrap`}>{t("keys.colFile")}</SortableTableHeader>
          <SortableTableHeader column="kind" {...keySort.headerProps} className={`${tableHeadCell} w-[18%] whitespace-nowrap`}>{t("keys.colKind")}</SortableTableHeader>
          <SortableTableHeader column="state" {...keySort.headerProps} className={`${tableHeadCell} w-[20%] whitespace-nowrap`}>{t("keys.colState")}</SortableTableHeader>
          <th scope="col" className={`${tableHeadCell} whitespace-nowrap text-right`}>{t("keys.colActions")}</th>
        </tr>
      </thead>
      <tbody className="block md:table-row-group">
        {displayed.map(({ item, relatedCount, relatedExpanded, relatedTo }) => {
          const heldByAgent = agentHolds(inventory, item);
          const isSelected = selected === item.id;
          const detailsExpanded = detailsFor === item.id;
          return (
            <Fragment key={item.id}>
            <tr
              data-key-related-to={relatedTo ?? undefined}
              className={`grid grid-cols-[2.25rem_minmax(0,1fr)] border-b border-hairline align-top transition-colors last:border-b-0 md:table-row ${
                isSelected ? "bg-select-fill" : ""
              } ${relatedTo === null ? "" : "bg-surface-subtle"}`}
            >
              <td className="row-span-3 py-3 pl-2 md:table-cell md:pl-3">
                {renameable(item, inventory.items) ? (
                  <div className="flex flex-col items-center gap-1 md:flex-row">
                    <label className="grid min-h-10 min-w-8 place-items-center md:min-h-0 md:min-w-0">
                      <input
                        type="checkbox"
                        aria-label={t("keys.chooseKey", { path: item.relativePath })}
                        checked={chosen.has(item.id)}
                        onChange={(event) => actions.onToggleChosen(item, event.target.checked)}
                        className="h-4 w-4 accent-accent"
                      />
                    </label>
                    <span
                      draggable
                      aria-label={t("keys.dragKey", { path: item.relativePath })}
                      onDragStart={(event) => actions.onBeginDrag(event, item)}
                      onDragEnd={actions.onEndDrag}
                      className="flex min-h-10 min-w-8 cursor-grab select-none items-center justify-center rounded px-1.5 py-1 text-sm leading-none text-ink-faint hover:bg-surface active:cursor-grabbing md:min-h-0 md:min-w-0"
                    >
                      ⠿
                    </span>
                  </div>
                ) : null}
              </td>
              <td className={`min-w-0 py-3 pr-3 md:table-cell md:pr-4 ${relatedTo === null ? "" : "pl-3 md:pl-6"}`}>
                <button
                  type="button"
                  aria-pressed={isSelected}
                  onClick={() => actions.onSelect(item)}
                  className="block min-h-10 max-w-full break-all text-left font-mono text-sm font-semibold text-ink underline-offset-4 hover:text-accent hover:underline md:min-h-0 md:break-normal"
                >
                  {item.relativePath}
                </button>
                {item.fingerprint === "" ? null : (
                  <p className="mt-1 max-w-xs truncate font-mono text-[11px] text-ink-muted" title={item.fingerprint}>
                    {item.fingerprint}
                  </p>
                )}
                {item.comment === "" ? null : (
                  <p className="mt-0.5 max-w-xs truncate text-xs text-ink-muted">{item.comment}</p>
                )}
                {relatedCount === 0 ? null : (
                  <button
                    type="button"
                    aria-expanded={relatedExpanded}
                    disabled={revealRelated}
                    onClick={() => toggleRelated(item)}
                    className="mt-2 inline-flex min-h-10 items-center gap-1.5 rounded-md bg-surface px-2 py-1 text-xs font-medium text-ink-muted hover:text-ink disabled:cursor-default md:min-h-0"
                  >
                    <DisclosureChevron expanded={relatedExpanded} className="size-3.5 text-ink-faint" />
                    {t("keys.relatedPublicFiles", { count: relatedCount })}
                  </button>
                )}
              </td>
              <td className="col-start-2 min-w-0 pb-3 pr-3 md:table-cell md:py-3 md:pr-4">
                <span className="inline-flex rounded-md bg-surface px-2 py-1 text-xs text-ink-muted">
                  {describeKeyKind(item.kind, t)}
                </span>
                {item.algorithm === "" ? null : (
                  <p className="mt-2 font-mono text-xs font-medium text-ink">
                    {item.bits > 0 ? `${item.algorithm} · ${item.bits}` : item.algorithm}
                  </p>
                )}
                {item.certificate === undefined ? null : (
                  <ul className="mt-1 text-xs text-ink-muted">
                    {certificateLines(item.certificate, now, t).map((line) => (
                      <li key={line.text} className={line.expired ? "text-danger" : undefined}>
                        {line.text}
                      </li>
                    ))}
                  </ul>
                )}
              </td>
              <td className="col-start-2 min-w-0 pb-3 pr-3 text-xs md:table-cell md:py-3 md:pr-4">
                <span className="flex flex-wrap gap-1.5">
                  {item.permissionRisk && (
                    <span className="rounded-md bg-notice px-2 py-1 font-medium text-notice-ink">
                      {t("keys.permissionRisk")}
                    </span>
                  )}
                  {heldByAgent && (
                    <span className="inline-flex items-center gap-1.5 rounded-md bg-surface px-2 py-1 font-medium text-live">
                      <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-live" />
                      {t("keys.stateInAgent")}
                    </span>
                  )}
                  {item.references.length > 0 && (
                    <span className="rounded-md bg-surface px-2 py-1 text-ink-muted">
                      {t("keys.stateUsedBy", { count: item.references.length })}
                    </span>
                  )}
                  {item.notes.map((note) => (
                    <span key={note} className="rounded-md bg-notice px-2 py-1 text-notice-ink">
                      {note in noteLabels ? t(noteLabels[note]!) : note}
                    </span>
                  ))}
                </span>
              </td>
              <td className="col-span-2 border-t border-hairline p-3 md:table-cell md:border-t-0 md:py-3 md:pr-3 md:pl-0">
                <button
                  type="button"
                  className={`${rowAction} ml-auto`}
                  aria-expanded={detailsExpanded}
                  onClick={() => actions.onToggleDetails(item)}
                >
                  <DisclosureChevron expanded={detailsExpanded} className="mr-1 size-3.5 text-ink-faint" />
                  {t(detailsExpanded ? "keys.hideDetails" : "keys.showDetails")}
                </button>
              </td>
            </tr>
            {detailsExpanded ? (
              <tr
                data-key-detail-for={item.id}
                className="block border-b border-line bg-card md:table-row"
              >
                <td colSpan={5} className="block p-3 pt-0 md:table-cell md:p-4 md:pt-1">
                  <section
                    aria-label={t("keys.actionsHeading", { path: item.relativePath })}
                    className="rounded-md border border-line bg-surface-subtle p-3 sm:p-4"
                  >
                    <div
                      role="group"
                      aria-label={t("keys.keyActions")}
                      className="flex flex-wrap items-center gap-2"
                    >
                      {(item.kind === "public_key" || item.kind === "certificate") && (
                        <button type="button" className={rowPrimary} onClick={() => actions.onShowPublicKey(item)}>
                          {t("keys.showPublicKey")}
                        </button>
                      )}
                      {item.kind === "private_key" && (
                        <>
                          <button type="button" className={rowAction} onClick={() => actions.onReveal(item)}>
                            {t("keys.showPrivateKey")}
                          </button>
                          <button
                            type="button"
                            className={heldByAgent ? rowAction : rowPrimary}
                            disabled={!inventory.agentAvailable}
                            onClick={() => heldByAgent ? actions.onRemoveFromAgent(item) : actions.onAddToAgent(item)}
                          >
                            {t(heldByAgent ? "keys.removeFromAgent" : "keys.addToAgent")}
                          </button>
                          {item.encrypted ? (
                            <button
                              type="button"
                              className={rowAction}
                              onClick={() => actions.onManageStoredPassphrase(item)}
                            >
                              {t("keys.manageStoredPassphrase")}
                            </button>
                          ) : null}
                          <button
                            type="button"
                            className={rowAction}
                            onClick={() => actions.onChangePassphrase(item)}
                          >
                            {t("keys.changePassphrase")}
                          </button>
                        </>
                      )}
                      {renameable(item, inventory.items) ? (
                        <button
                          type="button"
                          className={rowAction}
                          onClick={() => actions.onRelocate(item)}
                        >
                          {t("keys.relocate")}
                        </button>
                      ) : null}
                      {item.kind === "private_key" ? (
                        <button
                          type="button"
                          className={`${rowDanger} sm:ml-auto`}
                          onClick={() => actions.onMoveToTrash(item)}
                        >
                          {t("keys.moveToTrash")}
                        </button>
                      ) : null}
                    </div>
                    {inlinePanel?.itemId === item.id ? (
                      <div className="mt-3 border-t border-line pt-3">
                        {inlinePanel.content}
                      </div>
                    ) : null}
                  </section>
                </td>
              </tr>
            ) : null}
            </Fragment>
          );
        })}
        {displayed.length === 0 && (
          <tr className="block md:table-row">
            <td colSpan={5} className="block p-8 text-center text-sm text-ink-muted md:table-cell">
              {inventory.items.length === 0 ? t("keys.inventoryEmpty") : t("keys.noMatches")}
            </td>
          </tr>
        )}
      </tbody>
    </table>
  );
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
    const expiry = new Date(certificate.validBefore * 1000);
    const expired = certificate.validBefore * 1000 <= now;
    const when = `${expiry.toISOString().slice(0, 16).replace("T", " ")}Z`;
    lines.push({ text: expired ? t("keys.certExpired", { when }) : t("keys.certValidUntil", { when }), expired });
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
