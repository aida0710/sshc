import { useState, type DragEvent } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import type { KeyCertificate, KeyInventoryResponse, KeyItem } from "./api";
import { tableHeadCell, tableHeadRow } from "../ui/form";
import { noteLabels, rowAction, rowDanger, rowPrimary } from "./labels";
import { keyItemGroups } from "./organizer";

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
  onToggleMoreActions: (item: KeyItem) => void;
  onChangePassphrase: (item: KeyItem) => void;
  onRelocate: (item: KeyItem) => void;
  onMoveToTrash: (item: KeyItem) => void;
};

export function KeyTable({
  items,
  inventory,
  chosen,
  selected,
  moreActionsFor,
  now,
  revealRelated = false,
  actions,
}: {
  items: KeyItem[];
  inventory: KeyInventoryResponse;
  chosen: ReadonlySet<string>;
  selected: string | null;
  moreActionsFor: string;
  now: number;
  revealRelated?: boolean;
  actions: KeyRowActions;
}) {
  const t = useTranslate();
  const [expandedRelated, setExpandedRelated] = useState<ReadonlySet<string>>(new Set());
  const grouped = keyItemGroups(items);
  const displayed = grouped.flatMap((group) => {
    const relatedExpanded = revealRelated || expandedRelated.has(group.primary.id);
    return [
      {
        item: group.primary,
        relatedCount: group.related.length,
        relatedExpanded,
        relatedTo: null as string | null,
      },
      ...(relatedExpanded
        ? group.related.map((item) => ({
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
          <th scope="col" className={`${tableHeadCell} w-[30%] whitespace-nowrap`}>{t("keys.colFile")}</th>
          <th scope="col" className={`${tableHeadCell} w-[18%] whitespace-nowrap`}>{t("keys.colKind")}</th>
          <th scope="col" className={`${tableHeadCell} w-[20%] whitespace-nowrap`}>{t("keys.colState")}</th>
          <th scope="col" className={`${tableHeadCell} whitespace-nowrap text-right`}>{t("keys.colActions")}</th>
        </tr>
      </thead>
      <tbody className="block md:table-row-group">
        {displayed.map(({ item, relatedCount, relatedExpanded, relatedTo }) => {
          const heldByAgent = agentHolds(inventory, item);
          const isSelected = selected === item.id;
          return (
            <tr
              key={item.id}
              data-key-related-to={relatedTo ?? undefined}
              className={`grid grid-cols-[2.25rem_minmax(0,1fr)] border-b border-hairline align-top transition-colors last:border-b-0 hover:bg-select-fill md:table-row ${
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
                    <span aria-hidden="true" className="font-mono text-ink-faint">{relatedExpanded ? "▾" : "▸"}</span>
                    {t("keys.relatedPublicFiles", { count: relatedCount })}
                  </button>
                )}
              </td>
              <td className="col-start-2 min-w-0 pb-3 pr-3 md:table-cell md:py-3 md:pr-4">
                <span className="inline-flex rounded-md bg-surface px-2 py-1 text-xs text-ink-muted">
                  {item.kind}
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
                <div className="flex flex-wrap justify-start gap-2 md:justify-end md:gap-1 [&>button]:min-h-10 md:[&>button]:min-h-0">
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
                        className={heldByAgent ? rowAction : rowPrimary}
                        disabled={!inventory.agentAvailable}
                        onClick={() => actions.onAddToAgent(item)}
                      >
                        {t("keys.addToAgent")}
                      </button>
                      {heldByAgent && (
                        <button
                          type="button"
                          className={rowAction}
                          onClick={() => actions.onRemoveFromAgent(item)}
                        >
                          {t("keys.removeFromAgent")}
                        </button>
                      )}
                      <button
                        type="button"
                        className={rowAction}
                        aria-expanded={moreActionsFor === item.id}
                        onClick={() => actions.onToggleMoreActions(item)}
                      >
                        {t("keys.moreActions")}
                      </button>
                      {moreActionsFor === item.id ? (
                        <div className="mt-1 flex basis-full flex-wrap justify-start gap-2 rounded-lg bg-surface-subtle p-1.5 md:justify-end md:gap-1 [&>button]:min-h-10 md:[&>button]:min-h-0">
                          <button
                            type="button"
                            className={rowAction}
                            onClick={() => actions.onChangePassphrase(item)}
                          >
                            {t("keys.changePassphrase")}
                          </button>
                          {renameable(item, inventory.items) ? (
                            <button
                              type="button"
                              className={rowAction}
                              onClick={() => actions.onRelocate(item)}
                            >
                              {t("keys.relocate")}
                            </button>
                          ) : null}
                          <button
                            type="button"
                            className={rowDanger}
                            onClick={() => actions.onMoveToTrash(item)}
                          >
                            {t("keys.moveToTrash")}
                          </button>
                        </div>
                      ) : null}
                    </>
                  )}
                  {item.kind !== "private_key" && renameable(item, inventory.items) && (
                    <button
                      type="button"
                      className={rowAction}
                      onClick={() => actions.onRelocate(item)}
                    >
                      {t("keys.relocate")}
                    </button>
                  )}
                </div>
              </td>
            </tr>
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
