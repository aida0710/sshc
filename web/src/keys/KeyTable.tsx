import type { DragEvent } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import type { KeyCertificate, KeyInventoryResponse, KeyItem } from "./api";
import { tableHeadCell, tableHeadRow } from "../ui/form";
import { noteLabels, rowAction, rowDanger } from "./labels";

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
  actions,
}: {
  items: KeyItem[];
  inventory: KeyInventoryResponse;
  chosen: ReadonlySet<string>;
  selected: string | null;
  moreActionsFor: string;
  now: number;
  actions: KeyRowActions;
}) {
  const t = useTranslate();
  return (
  <table className="w-full text-left text-sm">
    <caption className="sr-only">{t("keys.tableCaption")}</caption>
    <thead>
      <tr className={tableHeadRow}>
        <th scope="col" className={`${tableHeadCell} w-8`}>
          <span className="sr-only">{t("keys.colChoose")}</span>
        </th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFile")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colKind")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colState")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colActions")}</th>
      </tr>
    </thead>
    <tbody>
      {items.map((item) => (
        <tr
          key={item.id}
          className="border-b border-line align-top transition-colors last:border-b-0 hover:bg-select-fill"
        >
          <td className="py-3 pl-3">

            {renameable(item, inventory.items) ? (
              <div className="flex items-center gap-1">
                <input
                  type="checkbox"
                  aria-label={t("keys.chooseKey", { path: item.relativePath })}
                  checked={chosen.has(item.id)}
                  onChange={(event) => actions.onToggleChosen(item, event.target.checked)}
                />


                <span
                  draggable
                  aria-label={t("keys.dragKey", { path: item.relativePath })}
                  onDragStart={(event) => actions.onBeginDrag(event, item)}
                  onDragEnd={actions.onEndDrag}
                  className="flex cursor-grab select-none items-center rounded px-2 py-2 text-base leading-none text-ink-faint hover:bg-select-fill active:cursor-grabbing"
                >
                  ⠿
                </span>
              </div>
            ) : null}
          </td>
          <td className="py-3 pr-3">
            <button
              type="button"
              aria-pressed={selected === item.id}
              onClick={() => actions.onSelect(item)}
              className="text-left font-mono text-xs text-ink underline-offset-2 hover:underline"
            >
              {item.relativePath}
            </button>
          </td>
          <td className="py-2 pr-3">
            {item.kind}
            {item.certificate === undefined ? null : (
              <ul className="text-xs text-ink-muted">
                {certificateLines(item.certificate, now, t).map((line) => (
                  <li key={line.text} className={line.expired ? "text-danger" : undefined}>
                    {line.text}
                  </li>
                ))}
              </ul>
            )}
          </td>

          <td className="py-2 pr-3 text-xs">
            <span className="flex flex-wrap gap-2">
              {item.permissionRisk && <span className="text-notice-ink">{t("keys.permissionRisk")}</span>}
              {agentHolds(inventory, item) && <span className="text-live">{t("keys.stateInAgent")}</span>}
              {item.references.length > 0 && (
                <span className="text-ink-muted">{t("keys.stateUsedBy", { count: item.references.length })}</span>
              )}
              {item.notes.map((note) => (
                <span key={note} className="text-notice-ink">
                  {note in noteLabels ? t(noteLabels[note]!) : note}
                </span>
              ))}
            </span>
          </td>
          <td className="py-2">
            <div className="flex flex-wrap gap-1">
            {(item.kind === "public_key" || item.kind === "certificate") && (
              <button type="button" className={rowAction} onClick={() => actions.onShowPublicKey(item)}>
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
                  className={rowAction}
                  disabled={!inventory.agentAvailable}
                  onClick={() => actions.onAddToAgent(item)}
                >
                  {t("keys.addToAgent")}
                </button>
                {agentHolds(inventory, item) && (
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
                  <div className="flex flex-wrap gap-1 rounded-md border border-line bg-surface-subtle p-1">
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
      ))}
      {items.length === 0 && (
        <tr>
          <td colSpan={5} className="p-5 text-sm text-ink-muted">
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
