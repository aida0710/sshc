import { useTranslate } from "../i18n/context";
import type { Credential } from "../api/integrations";
import type { KeyItem, RelocateKeyResponse } from "./api";
import {
  CheckboxField,
  Field,
  control,
  hintText,
  primaryAction,
  secondaryAction,
  sectionCard,
  sectionHeading,
} from "../ui/form";
import { Card, Row } from "../ui/surface";
import type {
  useAgentForm,
  usePassphraseForm,
  useRelocateForm,
  useStoredPassphraseForm,
  useStoredPhrases,
} from "./forms";
import { describeBlocker, noteLabels, rowDanger } from "./labels";

// 鍵の画面が開くフォームである。
//
// **prop の束は hook の戻り値そのものである。** ワークフローごとに状態をまとめた
// 結果、そのフォームが要るものは既に 1 つの値になっている——ここで 10 個の prop に
// 開き直す理由が無い。以前これらは KeysScreen の 1600 行の中に並んでおり、どの入力が
// どのフォームのものかは、閉じ括弧を数えないと分からなかった。

// namedStoredFor / dedicatedStoredFor は、保管庫がこの鍵の答えを持っているかを見る。
//
// **施錠された保管庫は何も答えない。** それでもフィールドを空欄のままにしてよいとは
// 判断できる。
function namedStoredFor(phrases: Credential[], item: KeyItem): Credential | undefined {
  return phrases.find((credential) => credential.uses.includes(item.relativePath));
}

function dedicatedStoredFor(paths: string[], item: KeyItem): boolean {
  return paths.includes(item.relativePath);
}

function hasStoredFor(phrases: Credential[], dedicatedPaths: string[], item: KeyItem): boolean {
  return namedStoredFor(phrases, item) !== undefined || dedicatedStoredFor(dedicatedPaths, item);
}

// RelocateForm は、鍵の名前を変える／別のグループへ移す。
export function RelocateForm({
  form,
  groups,
  onSubmit,
}: {
  form: ReturnType<typeof useRelocateForm>;
  groups: string[];
  onSubmit: (item: KeyItem) => void;
}) {
  const t = useTranslate();
  const item = form.relocating;
  if (item === null) return null;
  return (
    <form
      aria-labelledby="relocate-heading"
      className="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(item);
      }}
    >
      <h3 id="relocate-heading" className={sectionHeading}>
        {t("keys.relocateHeading", { path: item.relativePath })}
      </h3>
      <p className="text-sm text-ink-muted">{t("keys.relocateNote")}</p>
      <Card>
        <Row label={t("keys.relocateNewName")}>
          <input className={control} value={form.newName} onChange={(event) => form.setNewName(event.target.value)} />
        </Row>
        <Row label={t("keys.relocateGroup")}>
          <select className={control} value={form.newGroup} onChange={(event) => form.setNewGroup(event.target.value)}>
            <option value="">{t("keys.groupNone")}</option>
            {groups.map((group) => (
              <option key={group} value={group}>
                {group}
              </option>
            ))}
          </select>
        </Row>
      </Card>
      <div className="flex gap-2">
        <button type="submit" className={primaryAction}>
          {t("keys.relocateSubmit")}
        </button>
        <button type="button" className={secondaryAction} onClick={form.close}>
          {t("keys.cancel")}
        </button>
      </div>
    </form>
  );
}

// PassphraseForm は、鍵のパスフレーズを変える／外す。
//
// **入力は 1 回の送信の間だけここに居る。** 成功しても失敗しても消える。
export function PassphraseForm({
  form,
  onSubmit,
}: {
  form: ReturnType<typeof usePassphraseForm>;
  onSubmit: (item: KeyItem) => void;
}) {
  const t = useTranslate();
  const item = form.changingPassphrase;
  if (item === null) return null;
  return (
    <form
      aria-labelledby="passphrase-heading"
      className="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(item);
      }}
    >
      <h3 id="passphrase-heading" className={sectionHeading}>
        {t("keys.passphraseHeading", { path: item.relativePath })}
      </h3>
      <p className="text-sm text-ink-muted">{t("keys.passphraseNote")}</p>
      <Card>
        <Row label={t("keys.currentPassphrase")}>
          <input
            className={control}
            type="password"
            value={form.currentPassphrase}
            onChange={(event) => form.setCurrentPassphrase(event.target.value)}
          />
        </Row>
        <Row label={t("keys.newPassphrase")}>
          <input
            className={control}
            type="password"
            value={form.newPassphrase}
            onChange={(event) => form.setNewPassphrase(event.target.value)}
            disabled={form.removePassphrase}
          />
        </Row>
      </Card>
      <CheckboxField
        label={t("keys.removePassphrase")}
        checked={form.removePassphrase}
        onChange={(checked) => {
          form.setRemovePassphrase(checked);
          form.setNewPassphrase("");
        }}
      />
      <div className="flex gap-2">
        <button type="submit" className={primaryAction}>
          {t("keys.savePassphrase")}
        </button>
        <button type="button" className={secondaryAction} onClick={form.close}>
          {t("keys.cancel")}
        </button>
      </div>
    </form>
  );
}

// AgentForm は、鍵を ssh-agent へ登録する。
export function AgentForm({
  form,
  storedPhrases,
  onSubmit,
  onAssignPhrase,
}: {
  form: ReturnType<typeof useAgentForm>;
  storedPhrases: ReturnType<typeof useStoredPhrases>;
  onSubmit: (item: KeyItem) => void;
  onAssignPhrase: (item: KeyItem) => void;
}) {
  const t = useTranslate();
  const item = form.registering;
  if (item === null) return null;
  const { phrases, dedicatedPhrasePaths, chosenPhrase, setChosenPhrase } = storedPhrases;
  const stored = hasStoredFor(phrases, dedicatedPhrasePaths, item);
  return (
    <form
      aria-labelledby="agent-register-heading"
      className="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(item);
      }}
    >
      <h3 id="agent-register-heading" className={sectionHeading}>
        {t("keys.registerHeading", { path: item.relativePath })}
      </h3>
      <p className="text-sm text-ink-muted">{t("keys.registerNote")}</p>
      <Card>
        {item.encrypted && (
          // 「Passphrase」ではなく「Key passphrase」: 生成フォームにはそれ自身の
          // フィールドがあり、同じ名前の 2 つのコントロールでは見分けられない。
          <Row label={t("keys.keyPassphrase")} {...(!stored ? {} : { hint: t("keys.typedWins") })}>
            <input
              className={control}
              type="password"
              value={form.agentPassphrase}
              onChange={(event) => form.setAgentPassphrase(event.target.value)}
            />
          </Row>
        )}
        <Row label={t("keys.lifetime")}>
          <select
            className={control}
            value={String(form.agentLifetime)}
            onChange={(event) => form.setAgentLifetime(Number(event.target.value))}
          >
            <option value="0">{t("keys.lifetimeForever")}</option>
            <option value="3600">{t("keys.lifetimeHour")}</option>
            <option value="14400">{t("keys.lifetimeFourHours")}</option>
            <option value="43200">{t("keys.lifetimeTwelveHours")}</option>
          </select>
        </Row>
      </Card>
      {/*
        保存されたパスフレーズは、鍵の追加を 2 アクションではなく 1 アクションに変える。
        **ここに現れるのは鍵のパスフレーズだけである** ——このピッカーでアカウント
        パスワードを提供すれば、リモートホストのログイン資格情報をローカルの鍵に渡す
        ことになる。だからこそ保管庫は 2 つの名前空間を分けている。
      */}
      {item.encrypted && stored && (
        <p className={hintText}>
          {dedicatedStoredFor(dedicatedPhrasePaths, item)
            ? t("keys.usesDedicatedPassphrase")
            : t("keys.usesStoredPassphrase", { name: namedStoredFor(phrases, item)!.name })}
        </p>
      )}
      {item.encrypted && phrases.length > 0 && (
        <div className="flex flex-wrap items-end gap-3">
          <Row label={t("keys.useStoredPassphrase")}>
            <select className={control} value={chosenPhrase} onChange={(event) => setChosenPhrase(event.target.value)}>
              <option value="">{t("keys.choosePassphraseName")}</option>
              {phrases.map((credential) => (
                <option key={credential.name} value={credential.name}>
                  {credential.name}
                </option>
              ))}
            </select>
          </Row>
          <button
            type="button"
            className={secondaryAction}
            disabled={chosenPhrase === ""}
            onClick={() => onAssignPhrase(item)}
          >
            {t("keys.useThisPassphrase")}
          </button>
        </div>
      )}
      <div className="flex gap-2">
        <button type="submit" className={primaryAction}>
          {t("keys.registerSubmit")}
        </button>
        <button type="button" className={secondaryAction} onClick={form.close}>
          {t("keys.cancel")}
        </button>
      </div>
    </form>
  );
}

// StoredPassphrasePanel は、鍵に保管庫のパスフレーズを割り当てる。
export function StoredPassphrasePanel({
  form,
  storedPhrases,
  onAssign,
  onUnassign,
  onStoreAndAssign,
}: {
  form: ReturnType<typeof useStoredPassphraseForm>;
  storedPhrases: ReturnType<typeof useStoredPhrases>;
  onAssign: (item: KeyItem) => void;
  onUnassign: (item: KeyItem) => void;
  onStoreAndAssign: (item: KeyItem) => void;
}) {
  const t = useTranslate();
  const item = form.managingPassphrase;
  if (item === null) return null;
  const { phrases, dedicatedPhrasePaths, chosenPhrase, setChosenPhrase } = storedPhrases;
  return (
    <section aria-labelledby="stored-passphrase-heading" className={sectionCard}>
      <h3 id="stored-passphrase-heading" className={sectionHeading}>
        {t("keys.storedPassphraseHeading", { path: item.relativePath })}
      </h3>
      <p className={hintText}>{t("keys.storedPassphraseNote")}</p>
      {!hasStoredFor(phrases, dedicatedPhrasePaths, item) ? null : (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-3">
          <p className="grow text-sm text-ink">
            {dedicatedStoredFor(dedicatedPhrasePaths, item)
              ? t("keys.usesDedicatedPassphrase")
              : t("keys.usesStoredPassphrase", { name: namedStoredFor(phrases, item)!.name })}
          </p>
          <button type="button" className={secondaryAction} onClick={() => onUnassign(item)}>
            {t("keys.unassignPassphrase")}
          </button>
        </div>
      )}

      {phrases.length === 0 ? null : (
        <div className="flex flex-wrap items-end gap-3">
          <Field label={t("keys.useStoredPassphrase")}>
            <select className={control} value={chosenPhrase} onChange={(event) => setChosenPhrase(event.target.value)}>
              <option value="">{t("keys.choosePassphraseName")}</option>
              {phrases.map((credential) => (
                <option key={credential.name} value={credential.name}>{credential.name}</option>
              ))}
            </select>
          </Field>
          <button
            type="button"
            className={secondaryAction}
            disabled={chosenPhrase === ""}
            onClick={() => onAssign(item)}
          >
            {t("keys.useThisPassphrase")}
          </button>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("keys.newStoredPassphraseName")}>
          <input
            value={form.storedPhraseName}
            onChange={(event) => form.setStoredPhraseName(event.target.value)}
            className={control}
          />
        </Field>
        <Field label={t("keys.newStoredPassphraseValue")}>
          <input
            type="password"
            value={form.storedPhraseSecret}
            onChange={(event) => form.setStoredPhraseSecret(event.target.value)}
            className={control}
          />
        </Field>
      </div>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={form.storedPhraseName === "" || form.storedPhraseSecret === ""}
          className={primaryAction}
          onClick={() => onStoreAndAssign(item)}
        >
          {t("keys.storeAndUsePassphrase")}
        </button>
        <button type="button" className={secondaryAction} onClick={form.close}>
          {t("keys.cancel")}
        </button>
      </div>
    </section>
  );
}

// TrashConfirmation は、鍵をごみ箱へ移す前に、何が一緒に動くかを言う。
//
// **公開鍵が秘密鍵の隣から消えても驚かないように。** 両者は 1 つの鍵だからこそ
// 一緒に移動する。この鍵を名指すホストもここで挙げる——存在しないファイルを指す
// IdentityFile は、ssh が報告した上でそのまま続行してしまうものである。
export function TrashConfirmation({
  item,
  members,
  onConfirm,
  onCancel,
}: {
  item: KeyItem | null;
  members: KeyItem[];
  onConfirm: (id: string) => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  if (item === null) return null;
  return (
    <section aria-labelledby="trash-confirm-heading" className={sectionCard}>
      <h3 id="trash-confirm-heading" className={sectionHeading}>
        {t("keys.trashConfirmHeading", { path: item.relativePath })}
      </h3>
      <p className="text-sm text-ink">{t("keys.trashExplain")}</p>
      <ul className="flex flex-col gap-0.5 font-mono text-xs text-ink-muted">
        {members.map((member) => (
          <li key={member.id}>{member.relativePath}</li>
        ))}
      </ul>
      {item.references.length === 0 ? (
        <p className={hintText}>{t("keys.trashNoReferences")}</p>
      ) : (
        <>
          <p className="text-sm text-notice-ink">
            {t("keys.trashReferences", { count: item.references.length })}
          </p>
          <ul className="flex flex-col gap-0.5 font-mono text-xs text-notice-ink">
            {item.references.map((reference, index) => (
              <li key={`${reference.configPath}-${reference.line}-${index}`}>
                {`${reference.hostPatterns.join(" ")} — ${reference.configPath}:${reference.line}`}
              </li>
            ))}
          </ul>
        </>
      )}
      <p className={hintText}>{t("keys.trashIsRecoverable")}</p>
      <div className="flex flex-wrap gap-2">
        <button type="button" className={rowDanger} onClick={() => onConfirm(item.id)}>
          {t("keys.trashConfirm")}
        </button>
        <button type="button" className={secondaryAction} onClick={onCancel}>
          {t("keys.trashCancel")}
        </button>
      </div>
    </section>
  );
}

// RelocateResult は、鍵を動かそうとした結果である。
//
// **断られた relocation は、報告して忘れるべき失敗ではない。** サーバーは何も
// 書かずに理由を伝えたので、その理由は画面に残る。
export function RelocateResult({
  result,
  onClose,
}: {
  result: RelocateKeyResponse | null;
  onClose: () => void;
}) {
  const t = useTranslate();
  if (result === null) return null;
  return (
    <section aria-labelledby="relocate-result-heading" className="flex flex-col gap-2 text-sm">
      <h3 id="relocate-result-heading" className={sectionHeading}>
        {result.blockers.length > 0
          ? t("keys.relocateRefused")
          : t("keys.relocateDone", { path: result.relativePath })}
      </h3>
      {result.blockers.length > 0 && (
        <ul role="alert" className="text-notice-ink">
          {result.blockers.map((blocker) => (
            <li key={blocker}>{describeBlocker(blocker, t)}</li>
          ))}
        </ul>
      )}
      {result.files.length > 0 && (
        <>
          <h4 className="text-xs uppercase tracking-wide text-ink-muted">{t("keys.relocateMoved")}</h4>
          <ul className="font-mono text-xs text-ink-muted">
            {result.files.map((file) => (
              <li key={file.from}>{t("keys.relocateFilePair", { from: file.from, to: file.to })}</li>
            ))}
          </ul>
        </>
      )}
      {result.references.length > 0 && (
        <>
          <h4 className="text-xs uppercase tracking-wide text-ink-muted">{t("keys.relocateRewritten")}</h4>
          <ul className="text-xs text-ink-muted">
            {result.references.map((reference) => (
              <li key={`${reference.configPath}:${reference.line}:${reference.from}`}>
                {t("keys.relocateReference", {
                  directive: reference.directive,
                  from: reference.from,
                  to: reference.to,
                  path: reference.configPath,
                  line: reference.line,
                })}
              </li>
            ))}
          </ul>
        </>
      )}
      {result.skipped.length > 0 && (
        <p className="text-ink-muted">{t("keys.relocateSkipped", { paths: result.skipped.join(", ") })}</p>
      )}
      {result.notes.map((note) => (
        <p key={note} className="text-notice-ink">
          {note in noteLabels ? t(noteLabels[note]!) : note}
        </p>
      ))}
      <div>
        <button type="button" className={secondaryAction} onClick={onClose}>
          {t("keys.close")}
        </button>
      </div>
    </section>
  );
}
