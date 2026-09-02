import { useTranslate } from "../i18n/context";
import type { Credential } from "../api/integrations";
import type { KeyItem, RelocateKeyResponse } from "./api";
import {
  CheckboxField,
  Field,
  control,
  hintText,
  sectionCard,
  sectionHeading,
} from "../ui/form";
import { Button, Card, Row } from "../ui/surface";
import { PasswordInput } from "../ui/PasswordField";
import type {
  useAgentForm,
  usePassphraseForm,
  useRelocateForm,
  useStoredPassphraseForm,
  useStoredPhrases,
} from "./forms";
import { describeBlocker, noteLabels, rowDanger } from "./labels";

function namedStoredFor(
  phrases: Credential[],
  item: KeyItem,
): Credential | undefined {
  return phrases.find((credential) =>
    credential.uses.includes(item.relativePath),
  );
}

function dedicatedStoredFor(paths: string[], item: KeyItem): boolean {
  return paths.includes(item.relativePath);
}

function hasStoredFor(
  phrases: Credential[],
  dedicatedPaths: string[],
  item: KeyItem,
): boolean {
  return (
    namedStoredFor(phrases, item) !== undefined ||
    dedicatedStoredFor(dedicatedPaths, item)
  );
}

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
          <input
            className={control}
            value={form.newName}
            onChange={(event) => form.setNewName(event.target.value)}
          />
        </Row>
        <Row label={t("keys.relocateGroup")}>
          <select
            className={control}
            value={form.newGroup}
            onChange={(event) => form.setNewGroup(event.target.value)}
          >
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
        <Button kind="primary" type="submit">
          {t("keys.relocateSubmit")}
        </Button>
        <Button onClick={form.close}>{t("keys.cancel")}</Button>
      </div>
    </form>
  );
}

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
        <Row label={t("keys.currentPassphrase")} interactiveChildren>
          <PasswordInput
            label={t("keys.currentPassphrase")}
            value={form.currentPassphrase}
            onChange={form.setCurrentPassphrase}
          />
        </Row>
        <Row label={t("keys.newPassphrase")} interactiveChildren>
          <PasswordInput
            label={t("keys.newPassphrase")}
            value={form.newPassphrase}
            onChange={form.setNewPassphrase}
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
        <Button kind="primary" type="submit">
          {t("keys.savePassphrase")}
        </Button>
        <Button onClick={form.close}>{t("keys.cancel")}</Button>
      </div>
    </form>
  );
}

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
  const { phrases, dedicatedPhrasePaths, chosenPhrase, setChosenPhrase } =
    storedPhrases;
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
          <Row
            label={t("keys.keyPassphrase")}
            interactiveChildren
            {...(!stored ? {} : { hint: t("keys.typedWins") })}
          >
            <PasswordInput
              label={t("keys.keyPassphrase")}
              value={form.agentPassphrase}
              onChange={form.setAgentPassphrase}
            />
          </Row>
        )}
        <Row label={t("keys.lifetime")}>
          <select
            className={control}
            value={String(form.agentLifetime)}
            onChange={(event) =>
              form.setAgentLifetime(Number(event.target.value))
            }
          >
            <option value="0">{t("keys.lifetimeForever")}</option>
            <option value="3600">{t("keys.lifetimeHour")}</option>
            <option value="14400">{t("keys.lifetimeFourHours")}</option>
            <option value="43200">{t("keys.lifetimeTwelveHours")}</option>
          </select>
        </Row>
      </Card>

      {item.encrypted && stored && (
        <p className={hintText}>
          {dedicatedStoredFor(dedicatedPhrasePaths, item)
            ? t("keys.usesDedicatedPassphrase")
            : t("keys.usesStoredPassphrase", {
                name: namedStoredFor(phrases, item)!.name,
              })}
        </p>
      )}
      {item.encrypted && phrases.length > 0 && (
        <div className="flex flex-wrap items-end gap-3">
          <Row label={t("keys.useStoredPassphrase")}>
            <select
              className={control}
              value={chosenPhrase}
              onChange={(event) => setChosenPhrase(event.target.value)}
            >
              <option value="">{t("keys.choosePassphraseName")}</option>
              {phrases.map((credential) => (
                <option key={credential.name} value={credential.name}>
                  {credential.name}
                </option>
              ))}
            </select>
          </Row>
          <Button
            disabled={chosenPhrase === ""}
            onClick={() => onAssignPhrase(item)}
          >
            {t("keys.useThisPassphrase")}
          </Button>
        </div>
      )}
      <div className="flex gap-2">
        <Button kind="primary" type="submit">
          {t("keys.registerSubmit")}
        </Button>
        <Button onClick={form.close}>{t("keys.cancel")}</Button>
      </div>
    </form>
  );
}

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
  const { phrases, dedicatedPhrasePaths, chosenPhrase, setChosenPhrase } =
    storedPhrases;
  return (
    <section
      aria-labelledby="stored-passphrase-heading"
      className={sectionCard}
    >
      <h3 id="stored-passphrase-heading" className={sectionHeading}>
        {t("keys.storedPassphraseHeading", { path: item.relativePath })}
      </h3>
      <p className={hintText}>{t("keys.storedPassphraseNote")}</p>
      {!hasStoredFor(phrases, dedicatedPhrasePaths, item) ? null : (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-3">
          <p className="grow text-sm text-ink">
            {dedicatedStoredFor(dedicatedPhrasePaths, item)
              ? t("keys.usesDedicatedPassphrase")
              : t("keys.usesStoredPassphrase", {
                  name: namedStoredFor(phrases, item)!.name,
                })}
          </p>
          <Button onClick={() => onUnassign(item)}>
            {t("keys.unassignPassphrase")}
          </Button>
        </div>
      )}

      {phrases.length === 0 ? null : (
        <div className="flex flex-wrap items-end gap-3">
          <Field label={t("keys.useStoredPassphrase")}>
            <select
              className={control}
              value={chosenPhrase}
              onChange={(event) => setChosenPhrase(event.target.value)}
            >
              <option value="">{t("keys.choosePassphraseName")}</option>
              {phrases.map((credential) => (
                <option key={credential.name} value={credential.name}>
                  {credential.name}
                </option>
              ))}
            </select>
          </Field>
          <Button disabled={chosenPhrase === ""} onClick={() => onAssign(item)}>
            {t("keys.useThisPassphrase")}
          </Button>
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
        <Field label={t("keys.newStoredPassphraseValue")} interactiveChildren>
          <PasswordInput
            label={t("keys.newStoredPassphraseValue")}
            value={form.storedPhraseSecret}
            onChange={form.setStoredPhraseSecret}
          />
        </Field>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          kind="primary"
          disabled={
            form.storedPhraseName === "" || form.storedPhraseSecret === ""
          }
          onClick={() => onStoreAndAssign(item)}
        >
          {t("keys.storeAndUsePassphrase")}
        </Button>
        <Button onClick={form.close}>{t("keys.cancel")}</Button>
      </div>
    </section>
  );
}

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
                {`${reference.hostPatterns.join(" ")} · ${reference.configPath}:${reference.line}`}
              </li>
            ))}
          </ul>
        </>
      )}
      <p className={hintText}>{t("keys.trashIsRecoverable")}</p>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          className={rowDanger}
          onClick={() => onConfirm(item.id)}
        >
          {t("keys.trashConfirm")}
        </button>
        <Button onClick={onCancel}>{t("keys.trashCancel")}</Button>
      </div>
    </section>
  );
}

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
    <section
      aria-labelledby="relocate-result-heading"
      className="flex flex-col gap-2 text-sm"
    >
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
          <h4 className="text-xs uppercase tracking-wide text-ink-muted">
            {t("keys.relocateMoved")}
          </h4>
          <ul className="font-mono text-xs text-ink-muted">
            {result.files.map((file) => (
              <li key={file.from}>
                {t("keys.relocateFilePair", { from: file.from, to: file.to })}
              </li>
            ))}
          </ul>
        </>
      )}
      {result.references.length > 0 && (
        <>
          <h4 className="text-xs uppercase tracking-wide text-ink-muted">
            {t("keys.relocateRewritten")}
          </h4>
          <ul className="text-xs text-ink-muted">
            {result.references.map((reference) => (
              <li
                key={`${reference.configPath}:${reference.line}:${reference.from}`}
              >
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
        <p className="text-ink-muted">
          {t("keys.relocateSkipped", { paths: result.skipped.join(", ") })}
        </p>
      )}
      {result.notes.map((note) => (
        <p key={note} className="text-notice-ink">
          {note in noteLabels ? t(noteLabels[note]!) : note}
        </p>
      ))}
      <div>
        <Button onClick={onClose}>{t("keys.close")}</Button>
      </div>
    </section>
  );
}
