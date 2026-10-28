import type { Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { FieldRow, FieldRowNote, editorContainer } from "../ui/FieldRow";
import { control, Field, hintText } from "../ui/form";
import { Icon } from "../ui/icons";
import { PasswordInput } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import type { CreateConnectionAuthenticationKind, CreateConnectionDraft, CreationPrerequisite } from "./createConnectionDraft";
import { CreateConnectionVaultPrompt } from "./CreateConnectionVaultPrompt";
import { EditorSection } from "./EditorSection";
import { groupDefaultValue } from "./groupDefaults";
import { NewGroupField } from "./NewGroupField";
import { useCreateConnectionForm, type CreateConnectionFormOptions } from "./useCreateConnectionForm";
import { useReportDirty } from "./useReportDirty";

const authenticationChoices: readonly [CreateConnectionAuthenticationKind, MessageKey][] = [
  ["identity_file", "conn.createIdentityFile"],
  ["dedicated_password", "conn.createDedicatedPassword"],
  ["saved_password", "conn.createSavedPassword"],
  ["new_shared_password", "conn.createNewSharedPassword"],
];

type CreateConnectionPaneProps = CreateConnectionFormOptions & {
  groups: Overview["groups"];
  onCancel: () => void;
  // onCreateGroup は、グループを作れたかを返す。無ければ［新しいグループ］を出さない。
  onCreateGroup?: ((name: string) => Promise<boolean>) | undefined;
  onOpenPrerequisite?: ((section: CreationPrerequisite, draft: CreateConnectionDraft) => void) | undefined;
  onDirtyChange?: ((dirty: boolean) => void) | undefined;
};

// FieldProblem は、離れた欄の値の問題を、その欄の下に出す。フォーカスが次の欄へ移ったときに出るので、
// alert にはしない（移った先の操作を読み上げで遮らないため）。
function FieldProblem({ text }: { text: string }) {
  return text === "" ? null : <p className="mt-1 text-xs text-danger">{text}</p>;
}

// CreateConnectionPane は、Connections の右の欄で開く新しい接続の欄である。入力のためにほかの操作を止める
// モーダルにはせず、一覧やグループはそのまま触れる。作成するまで設定ファイルには何も書かない。
export function CreateConnectionPane({
  groups,
  onCancel,
  onCreateGroup,
  onOpenPrerequisite,
  onDirtyChange,
  ...formOptions
}: CreateConnectionPaneProps) {
  const t = useTranslate();
  const form = useCreateConnectionForm(formOptions);
  const { values, fieldErrors, touched } = form;
  useReportDirty(form.dirty, onDirtyChange);

  const defaultUser = values.group === "" ? "" : groupDefaultValue(groups, values.group, "User");
  const defaultPort = values.group === "" ? "" : groupDefaultValue(groups, values.group, "Port");
  // 欄の問題は、その欄から一度離れたあとで出す。打っている途中には出さない。
  const visibleProblem = (field: keyof typeof fieldErrors) => (touched.has(field) ? fieldErrors[field] : "");

  function cancel() {
    form.clearSecrets();
    form.clearError();
    onCancel();
  }

  function openPrerequisite(section: CreationPrerequisite) {
    const draft = form.safeDraft();
    form.clearSecrets();
    form.clearError();
    onOpenPrerequisite?.(section, draft);
  }

  const passwordNeedsVault = form.vault !== null && !form.vault.unlocked && values.authentication !== "identity_file";

  return (
    <form
      aria-labelledby="create-connection-heading"
      onSubmit={(event) => void form.submit(event)}
      className={`${editorContainer} flex min-h-0 flex-1 flex-col`}
    >
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <header className="mb-3.5 flex items-center gap-2.5">
          <span aria-hidden="true" className="flex size-[34px] shrink-0 items-center justify-center rounded-[9px] bg-select-fill text-accent">
            <Icon name="plus" className="size-[17px]" />
          </span>
          <div className="min-w-0">
            <h3 id="create-connection-heading" className="text-base font-semibold text-ink">{t("conn.createTitle")}</h3>
            <p className="text-xs text-ink-faint">{t("conn.createDescription")}</p>
          </div>
        </header>

        {form.error === "" ? null : <div className="mb-3.5"><Notice tone="danger">{form.error}</Notice></div>}

        <EditorSection title={t("conn.createNameSection")}>
          <div className="grid gap-2.5 @sm/editor:grid-cols-2">
            <div>
              <Field label={t("conn.createName")}>
                <input
                  id="create-connection-name"
                  required
                  autoFocus
                  value={values.alias}
                  onChange={(event) => form.setAlias(event.target.value)}
                  onBlur={() => form.touch("alias")}
                  aria-invalid={visibleProblem("alias") !== ""}
                  className={control}
                />
              </Field>
              <FieldProblem text={visibleProblem("alias")} />
            </div>
            <div>
              <Field label={t("conn.createGroup")}>
                <select id="create-connection-group" value={values.group} onChange={(event) => form.setGroup(event.target.value)} className={control}>
                  <option value="">{t("conn.createNoGroup")}</option>
                  {groups.map((candidate) => <option key={candidate.name} value={candidate.name}>{candidate.name}</option>)}
                </select>
              </Field>
              {/* 接続名の欄の横に置く。下に置くと、接続名から離れたときに出る問題の文で押す前に位置がずれる。 */}
              {onCreateGroup === undefined ? null : (
                <NewGroupField groupNames={groups.map((candidate) => candidate.name)} onCreate={onCreateGroup} onCreated={form.setGroup} />
              )}
            </div>
          </div>
        </EditorSection>

        <EditorSection title={t("conn.createConnectionSection")}>
          <div className="grid grid-cols-[minmax(0,1fr)_5.25rem] gap-2.5 @sm/editor:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_5.25rem]">
            <div className="col-span-2 @sm/editor:col-span-1">
              <Field label={t("conn.createHostName")}>
                <input
                  id="create-connection-hostname"
                  required
                  value={values.hostName}
                  onChange={(event) => form.setHostName(event.target.value)}
                  onBlur={() => form.touch("hostName")}
                  aria-invalid={visibleProblem("hostName") !== ""}
                  className={`${control} font-mono`}
                />
              </Field>
              <FieldProblem text={visibleProblem("hostName")} />
            </div>
            <div>
              <Field label={t("conn.createUser")}>
                <input
                  id="create-connection-user"
                  value={values.user}
                  placeholder={defaultUser}
                  onChange={(event) => form.setUser(event.target.value)}
                  onBlur={() => form.touch("user")}
                  aria-invalid={visibleProblem("user") !== ""}
                  className={`${control} font-mono`}
                />
              </Field>
              <FieldProblem text={visibleProblem("user")} />
            </div>
            <div>
              <Field label={t("conn.createPort")}>
                <input
                  id="create-connection-port"
                  type="number"
                  min={1}
                  max={65535}
                  value={values.port}
                  placeholder={defaultPort === "" ? "22" : defaultPort}
                  onChange={(event) => form.setPort(event.target.value)}
                  onBlur={() => form.touch("port")}
                  aria-invalid={visibleProblem("port") !== ""}
                  className={`${control} font-mono`}
                />
              </Field>
              <FieldProblem text={visibleProblem("port")} />
            </div>
          </div>
          {defaultUser === "" && defaultPort === "" ? null : (
            <p className={`mt-1.5 ${hintText}`}>{t("conn.createGroupDefaults", { group: values.group })}</p>
          )}
        </EditorSection>

        <EditorSection title={t("conn.createAuthenticationSection")}>
          {form.loading ? <p className={`mb-1.5 ${hintText}`}>{t("conn.createLoadingOptions")}</p> : null}
          <div>
            <FieldRow density="editor" label={t("conn.createAuthenticationMethod")}>
              <select
                value={values.authentication}
                disabled={form.loading}
                onChange={(event) => form.chooseAuthentication(event.target.value as CreateConnectionAuthenticationKind)}
                className={control}
              >
                {authenticationChoices.map(([kind, label]) => <option key={kind} value={kind}>{t(label)}</option>)}
              </select>
            </FieldRow>

            {form.vault !== null && passwordNeedsVault ? (
              <FieldRowNote density="editor">
                <CreateConnectionVaultPrompt
                  vault={form.vault}
                  masterPassword={values.masterPassword}
                  onMasterPasswordChange={form.setMasterPassword}
                  masterConfirmation={values.masterConfirmation}
                  onMasterConfirmationChange={form.setMasterConfirmation}
                  canOpen={form.canOpenVault}
                  busy={form.vaultBusy}
                  onOpen={() => void form.openVault()}
                />
              </FieldRowNote>
            ) : null}

            {values.authentication === "identity_file" ? (
              <>
                <FieldRow density="editor" label={t("conn.createPrivateKey")}>
                  <select value={values.keyId} onChange={(event) => form.setKeyId(event.target.value)} className={control}>
                    {form.privateKeys.length === 0 ? <option value="">{t("conn.createNoPrivateKeys")}</option> : null}
                    {form.privateKeys.map((key) => (
                      <option key={key.id} value={key.id}>{key.relativePath}{key.fingerprint === "" ? "" : ` · ${key.fingerprint}`}</option>
                    ))}
                  </select>
                </FieldRow>
                {form.loading || form.privateKeys.length > 0 ? null : (
                  <FieldRowNote density="editor">
                    <Button className="self-start" onClick={() => openPrerequisite("Keys")}>{t("conn.createCreatePrivateKey")}</Button>
                  </FieldRowNote>
                )}
              </>
            ) : passwordNeedsVault || form.vault === null ? null : values.authentication === "dedicated_password" ? (
              <FieldRow density="editor" label={t("conn.createConnectionPassword")} hint={t("conn.createDedicatedHint")} interactiveChildren>
                <PasswordInput label={t("conn.createConnectionPassword")} value={values.dedicatedPassword} onChange={form.setDedicatedPassword} />
              </FieldRow>
            ) : values.authentication === "saved_password" ? (
              <FieldRow density="editor" label={t("conn.createChooseSavedPassword")} hint={t("conn.createSavedHint")}>
                <select value={values.savedCredential} onChange={(event) => form.setSavedCredential(event.target.value)} className={control}>
                  {form.credentials.length === 0 ? <option value="">{t("conn.createNoSavedPasswords")}</option> : null}
                  {form.credentials.map((credential) => (
                    <option key={credential.name} value={credential.name}>{credential.name}</option>
                  ))}
                </select>
              </FieldRow>
            ) : (
              <>
                <FieldRow density="editor" label={t("conn.createSavedPasswordName")}>
                  <input value={values.newCredential} onChange={(event) => form.setNewCredential(event.target.value)} className={control} />
                </FieldRow>
                <FieldRow density="editor" label={t("conn.createNewPassword")} interactiveChildren>
                  <PasswordInput label={t("conn.createNewPassword")} value={values.newSharedPassword} onChange={form.setNewSharedPassword} />
                </FieldRow>
              </>
            )}
          </div>
        </EditorSection>
      </div>

      {/* 欄が狭いときは、作成できない理由をボタンの上の1行に出す。 */}
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
        {form.disabledReason === "" ? null : (
          <p className={`basis-full @sm/editor:mr-auto @sm/editor:basis-0 @sm/editor:grow ${hintText}`}>{form.disabledReason}</p>
        )}
        <Button disabled={form.busy || form.vaultBusy} onClick={cancel}>{t("conn.cancelCreate")}</Button>
        <Button type="submit" kind="primary" disabled={!form.canSubmit}>
          {form.busy ? t("conn.creating") : t("conn.create")}
        </Button>
      </div>
    </form>
  );
}
