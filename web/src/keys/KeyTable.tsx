import type { DragEvent } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import type { KeyCertificate, KeyInventoryResponse, KeyItem } from "./api";
import { tableHeadCell, tableHeadRow } from "../ui/form";
import { noteLabels, rowAction, rowDanger } from "./labels";

// KeyRowActions は、行から始められることの全部である。
//
// **setter ではなく、意図を渡す。** 「保管庫のパネルを開く」は、開く前に他の入力欄を
// 畳み、開いたあとに一覧を取り直すところまでを含む——その順序は画面の持ち物であって、
// 行が知っていてよいことではない。行が知っているのは、押されたということだけである。
export type KeyRowActions = {
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

// KeyTable は、いま見えている鍵の一覧である。
//
// **並べ方も絞り込みも、ここでは決めない。** どれを見せるかは画面が決めて渡す。
// 以前この表は KeysScreen の render の中に 217 行として置かれており、行が何を触るのか
// は閉じ括弧を数えないと分からなかった。
export function KeyTable({
  items,
  inventory,
  chosen,
  moreActionsFor,
  now,
  actions,
}: {
  items: KeyItem[];
  inventory: KeyInventoryResponse;
  chosen: ReadonlySet<string>;
  moreActionsFor: string;
  now: number;
  actions: KeyRowActions;
}) {
  const t = useTranslate();
  return (
  <table className="w-full min-w-[56rem] text-left text-sm">
    <caption className="sr-only">{t("keys.tableCaption")}</caption>
    <thead>
      <tr className={tableHeadRow}>
        <th scope="col" className={`${tableHeadCell} w-8`}>
          <span className="sr-only">{t("keys.colChoose")}</span>
        </th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFile")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colKind")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colAlgorithm")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFingerprint")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colPermissions")}</th>
        <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colUsedBy")}</th>
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
            {/* **一緒に動くものは、別々に選ばせない。** 秘密鍵を動かせば
                その公開鍵と証明書も付いていく（relocate がそうする）ので、
                ここで選べるのは relocate が名前を変えられるものだけである。 */}
            {renameable(item, inventory.items) ? (
              <div className="flex items-center gap-1">
                <input
                  type="checkbox"
                  aria-label={t("keys.chooseKey", { path: item.relativePath })}
                  checked={chosen.has(item.id)}
                  onChange={(event) => actions.onToggleChosen(item, event.target.checked)}
                />
                {/* **掴む場所を決める。** 行ごと掴めるようにすると、文字を
                    選ぼうとした指がそのまま鍵を運んでしまう。持てる場所が
                    目に見えている方が、掴んでよいと分かる。 */}
                {/* **掴む場所は大きくする。** 一文字ぶんの幅しかないと、
                    狙いを定めるだけで手が止まる。行の高さいっぱいを持てる
                    ようにして、掴んでよい場所が目で分かるようにする。 */}
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
          <td className="py-3 pr-3 font-mono text-xs">{item.relativePath}</td>
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
          <td className="py-2 pr-3">{item.bits > 0 ? `${item.algorithm} · ${item.bits}` : item.algorithm}</td>
          <td className="py-2 pr-3 font-mono text-xs break-all">
            {item.fingerprint !== "" ? item.fingerprint : null}
            {item.notes.map((note) => (
              <span key={note} className="ml-2 text-notice-ink">
                {note in noteLabels ? t(noteLabels[note]!) : note}
              </span>
            ))}
          </td>
          <td className="py-2 pr-3">
            {item.permission}
            {item.permissionRisk && <span className="ml-2 text-notice-ink">{t("keys.permissionRisk")}</span>}
          </td>
          <td className="py-2 pr-3">
            {item.references.map((reference) => reference.hostPatterns.join(" ")).join(", ")}
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
          <td colSpan={8} className="p-5 text-sm text-ink-muted">
            {inventory.items.length === 0 ? t("keys.inventoryEmpty") : t("keys.noMatches")}
          </td>
        </tr>
      )}
    </tbody>
  </table>
  );
}

// certificateLines は OpenSSH の証明書を、使えるかどうかを決める
// 観点で記述する: 誰を名指すか、誰のためのものか、期限が切れているか
// どうかだ。「certificate」とだけ言う期限切れの証明書は動作するものと
// 見分けがつかない。これが design §6.3 がそれらを分類する理由のすべてだ。
function certificateLines(
  certificate: KeyCertificate,
  now: number,
  t: Translate,
): { text: string; expired: boolean }[] {
  const lines: { text: string; expired: boolean }[] = [];
  if (certificate.keyId !== "") lines.push({ text: t("keys.certKeyId", { keyId: certificate.keyId }), expired: false });
  if (certificate.principals.length > 0) {
    lines.push({ text: t("keys.certFor", { principals: certificate.principals.join(", ") }), expired: false });
  } else {
    // principal のない証明書は、その CA を信頼するホスト上の全ユーザーに
    // 対して有効だ。これはフィールドの欠落ではなく、その及ぶ範囲についての事実だ。
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

// 行のアクションはルールのないテーブルの中のただのボタンだったので、
// テキストとして連なり、コントロールではなく文章として読めてしまっていた。
// renameable は、ユーザーが伝えていないことを決めずに、このアプリケーションが
// ファイルをリネームできるかどうかを報告する。
//
// 秘密鍵は自分の公開鍵と証明書を道連れにするので、常にリネーム
// 可能だ。公開鍵や証明書がリネーム可能なのは、インベントリ内の
// どの秘密鍵もそれに属していない場合のみだ: ペアの片方だけをリネームすると、
// OpenSSH がいまだに名前で対応付けている 2 つのファイルを、読み手が
// 対応付けられなくなるので、サーバーはそれを拒否し、ボタンも提供されない。
function renameable(item: KeyItem, items: KeyItem[]): boolean {
  if (item.kind === "private_key") return true;
  if (item.kind !== "public_key" && item.kind !== "certificate") return false;
  const fingerprint =
    item.kind === "certificate" && item.certificate !== undefined
      ? item.certificate.signedKeyFingerprint
      : item.fingerprint;
  if (fingerprint === "") return true;
  return !items.some((candidate) => candidate.kind === "private_key" && candidate.fingerprint === fingerprint);
}

// agentHolds は、エージェントが現在この鍵を保持しているかどうかを、
// フィンガープリントで照合して報告する。エージェントの identity とインベントリ
// 項目に共通するのはそれだけだ——エージェントはファイルパスを何も知らない。
function agentHolds(inventory: KeyInventoryResponse, item: KeyItem): boolean {
  if (!inventory.agentAvailable || item.fingerprint === "") return false;
  return inventory.agentIdentities.some((identity) => identity.fingerprint === item.fingerprint);
}
