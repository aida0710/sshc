import type {
  HostDetail,
  UpdateConnectionKeyPassphrase,
  UpdateConnectionPassword,
  UpdateConnectionRequest,
  UpdateConnectionTOTP,
} from "../api/config";
import type { KeyItem } from "../keys/api";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import { formatValues, isValidHostName } from "../rules/rules";
import { directIdentityFields, isConcreteIdentityValue } from "./authenticationPolicy";
import { deriveBasicField, type BasicFieldState, type BasicKeyword } from "./basicFields";
import type { Translate } from "../i18n/context";
import type { ConnectionSecretsModel } from "./useConnectionSecrets";

export type PasswordAction = UpdateConnectionPassword["kind"];
export type TOTPAction = UpdateConnectionTOTP["kind"];

export type DraftField = {
  state: BasicFieldState;
  value: string;
  inherit: boolean;
};

export function initialField(detail: HostDetail, keyword: BasicKeyword): DraftField {
  const state = deriveBasicField(detail, keyword);
  return { state, value: state.value, inherit: false };
}

// How the configured IdentityFile maps onto the key list: one known key
// that can be swapped, one path the list does not know, or several files
// that the basic form leaves to the advanced editor.
export type KeyState = "loading" | "editable" | "custom" | "complex";
export const customKeyId = "__custom__";

export type KeySelection = {
  keyState: KeyState;
  customKey: string;
  initialKey: string;
  // What the select shows before the user touches it.
  initialSelected: string;
  // The generated key the page asked to stage is the one already configured.
  preferredAlreadyApplied: boolean;
};

function keyConfigValue(key: KeyItem): string {
  return `~/.ssh/${key.relativePath}`;
}

export function keySelectionOf(
  detail: HostDetail,
  identities: KeyItem[],
  available: boolean,
  preferredKey: GeneratedPrivateKeyHandoff | null,
): KeySelection {
  const preferred = preferredKey === null
    ? undefined
    : identities.find((candidate) =>
        candidate.id === preferredKey.privateKeyId && candidate.relativePath === preferredKey.privateRelativePath);
  const direct = directIdentityFields(detail).filter((field) => field.values.some(isConcreteIdentityValue));
  if (direct.length > 1) {
    return { keyState: "complex", customKey: "", initialKey: "", initialSelected: "", preferredAlreadyApplied: false };
  }
  if (direct.length === 1) {
    const configured = formatValues(direct[0]!.values) ?? "";
    const matched = identities.find((candidate) => keyConfigValue(candidate) === configured);
    if (!available || matched === undefined) {
      return { keyState: "custom", customKey: configured, initialKey: customKeyId, initialSelected: customKeyId, preferredAlreadyApplied: false };
    }
    return {
      keyState: "editable",
      customKey: "",
      initialKey: matched.id,
      initialSelected: preferred?.id ?? matched.id,
      preferredAlreadyApplied: preferred?.id === matched.id,
    };
  }
  return {
    keyState: available ? "editable" : "loading",
    customKey: "",
    initialKey: "",
    initialSelected: available ? preferred?.id ?? "" : "",
    preferredAlreadyApplied: false,
  };
}

// Every value the user can type or pick. Secrets live here too, and are
// wiped whenever the form resets, saves or unmounts.
export type BasicDraft = {
  hostName: DraftField;
  user: DraftField;
  port: DraftField;
  selectedKey: string;
  passwordAction: PasswordAction;
  password: string;
  savedCredential: string;
  newCredential: string;
  newSharedPassword: string;
  confirmRemove: boolean;
  totpAction: TOTPAction;
  savedTOTP: string;
  keyPassphrase: string;
  keyPassphraseConfirmation: string;
  masterPassword: string;
  masterConfirmation: string;
};

export function initialDraft(detail: HostDetail, selectedKey = ""): BasicDraft {
  return {
    hostName: initialField(detail, "HostName"),
    user: initialField(detail, "User"),
    port: initialField(detail, "Port"),
    selectedKey,
    passwordAction: "unchanged",
    password: "",
    savedCredential: "",
    newCredential: "",
    newSharedPassword: "",
    confirmRemove: false,
    totpAction: "unchanged",
    savedTOTP: "",
    keyPassphrase: "",
    keyPassphraseConfirmation: "",
    masterPassword: "",
    masterConfirmation: "",
  };
}

export const clearedPasswordSecrets: Pick<BasicDraft, "password" | "newSharedPassword" | "masterPassword" | "masterConfirmation"> = {
  password: "", newSharedPassword: "", masterPassword: "", masterConfirmation: "",
};
export const clearedKeyPassphrase: Pick<BasicDraft, "keyPassphrase" | "keyPassphraseConfirmation"> = {
  keyPassphrase: "", keyPassphraseConfirmation: "",
};
export const clearedSecrets = { ...clearedPasswordSecrets, ...clearedKeyPassphrase };
// What a save or a password-action change puts back to "nothing chosen".
export const clearedPasswordChoice: Pick<BasicDraft, "passwordAction" | "confirmRemove" | "newCredential"> = {
  passwordAction: "unchanged", confirmRemove: false, newCredential: "",
};

type FieldChange = { action: "inherit" } | { action: "set"; value: string } | undefined;

function stringChange(field: DraftField, allowEmpty: boolean): FieldChange {
  if (!field.state.editable || field.state.origin === "complex") return undefined;
  if (field.inherit || (allowEmpty && field.value === "" && field.state.origin === "direct")) {
    return field.state.origin === "direct" ? { action: "inherit" } : undefined;
  }
  if (field.value === field.state.value) return undefined;
  return { action: "set", value: field.value };
}

// Reads the draft against what the vault and key list say, and answers
// every question the form asks: what changed, what is wrong, what is
// allowed and whether the whole thing can be saved.
export function deriveBasicForm(
  detail: HostDetail,
  draft: BasicDraft,
  keySelection: KeySelection,
  secrets: Pick<ConnectionSecretsModel, "privateKeys" | "keyOptionsStatus" | "vault" | "eligibility" | "credentials" | "credentialOptionsStatus" | "loading">,
  t: Translate,
) {
  const { hostName, user, port, selectedKey } = draft;
  const { keyState, initialKey } = keySelection;
  const { credentials, vault, eligibility, credentialOptionsStatus, keyOptionsStatus, privateKeys } = secrets;
  // A remembered choice is kept only while the vault still lists it.
  const savedCredential = credentials.passwords.some((credential) => credential.name === draft.savedCredential)
    ? draft.savedCredential
    : credentials.passwords[0]?.name ?? "";
  const savedTOTP = credentials.totps.some((credential) => credential.name === draft.savedTOTP)
    ? draft.savedTOTP
    : credentials.totps[0]?.name ?? "";

  const hostNameChange = stringChange(hostName, false);
  const userChange = stringChange(user, true);
  const portChange = port.inherit
    ? port.state.origin === "direct" ? { action: "inherit" as const } : undefined
    : port.value === port.state.value
      ? undefined
      : { action: "set" as const, value: Number(port.value) };
  const identityFileChange = keyState !== "editable" || selectedKey === initialKey
    ? undefined
    : selectedKey === ""
      ? { action: "inherit" as const }
      : { action: "set" as const, keyId: selectedKey };
  const selectedPrivateKey = privateKeys.find((key) => key.id === selectedKey);
  const draftHasExplicitKey = keyState === "custom" || keyState === "complex" ||
    (keyState === "editable" && selectedKey !== "");
  const passwordCleanup = credentials.assigned && draftHasExplicitKey;
  const authenticationRouteChanged = hostNameChange !== undefined || userChange !== undefined || portChange !== undefined;
  const confirmPasswordRoute = credentials.assigned && !draftHasExplicitKey && draft.passwordAction === "unchanged" &&
    (authenticationRouteChanged || eligibility?.passwordBinding === "stale");
  const confirmTOTPRoute = credentials.assignedTOTP !== "" && draft.totpAction === "unchanged" &&
    (authenticationRouteChanged || eligibility?.totpBinding === "stale");
  const namedKeyPassphrase = selectedPrivateKey === undefined
    ? undefined
    : credentials.keyPassphrases.find((credential) => credential.uses.includes(selectedPrivateKey.relativePath));
  const dedicatedKeyPassphrase = selectedPrivateKey !== undefined &&
    (vault?.dedicatedKeyPassphrases ?? []).includes(selectedPrivateKey.relativePath);
  const keyPassphraseStorageState = dedicatedKeyPassphrase
    ? "dedicated"
    : namedKeyPassphrase === undefined
      ? "none"
      : `named:${namedKeyPassphrase.name}`;
  const otherNamedKeyUses = namedKeyPassphrase === undefined || selectedPrivateKey === undefined
    ? []
    : namedKeyPassphrase.uses.filter((subject) => subject !== selectedPrivateKey.relativePath);

  const hostError = hostName.inherit
    ? ""
    : hostName.value === ""
      ? t("conn.createHostRequired")
      : isValidHostName(hostName.value)
        ? ""
        : t("conn.createHostInvalid");
  const userError = user.value !== "" && /[\s\p{Cc}]/u.test(user.value) ? t("conn.createUserInvalid") : "";
  const parsedPort = Number(port.value);
  const portError = port.inherit || (/^\d+$/.test(port.value) && parsedPort >= 1 && parsedPort <= 65535)
    ? ""
    : t("conn.createPortInvalid");

  const passwordChange = ((): UpdateConnectionPassword => {
    if (passwordCleanup) return { kind: "remove" };
    switch (draft.passwordAction) {
      case "dedicated_password":
        return draft.password === "" ? { kind: "unchanged" } : { kind: "dedicated_password", password: draft.password };
      case "saved_password":
        return savedCredential === "" ? { kind: "unchanged" } : { kind: "saved_password", credential: savedCredential };
      case "new_shared_password":
        return draft.newCredential === "" || draft.newSharedPassword === ""
          ? { kind: "unchanged" }
          : { kind: "new_shared_password", credential: draft.newCredential, password: draft.newSharedPassword };
      case "remove":
        return draft.confirmRemove && credentials.assigned ? { kind: "remove" } : { kind: "unchanged" };
      case "confirm_route":
        return { kind: "confirm_route" };
      case "unchanged":
        return confirmPasswordRoute ? { kind: "confirm_route" } : { kind: "unchanged" };
    }
  })();
  const keyPassphraseChange: UpdateConnectionKeyPassphrase =
    selectedPrivateKey !== undefined && selectedPrivateKey.encrypted && draft.keyPassphrase !== ""
      ? { kind: "set_dedicated", keyId: selectedPrivateKey.id, passphrase: draft.keyPassphrase }
      : { kind: "unchanged" };
  const totpChange: UpdateConnectionTOTP = draft.totpAction === "saved_totp"
    ? savedTOTP === ""
      ? { kind: "unchanged" }
      : { kind: "saved_totp", credential: savedTOTP }
    : draft.totpAction === "remove"
      ? { kind: "remove" }
      : draft.totpAction === "confirm_route" || confirmTOTPRoute
        ? { kind: "confirm_route" }
        : { kind: "unchanged" };
  const changesPassword = passwordChange.kind !== "unchanged";
  const changesTOTP = totpChange.kind !== "unchanged";
  const hasKeyPassphraseDraft = draft.keyPassphrase !== "" || draft.keyPassphraseConfirmation !== "";
  const keyPassphraseValid = !hasKeyPassphraseDraft ||
    (draft.keyPassphrase !== "" && draft.keyPassphrase === draft.keyPassphraseConfirmation);
  const nonIdentityBlockers = (eligibility?.blockers ?? []).filter((notice) => notice.code !== "identity_file_configured");
  const passwordWarnings = (eligibility?.warnings ?? []).filter((notice) => notice.code !== "identity_file_configured");
  const passwordAllowed = passwordChange.kind === "remove" || passwordChange.kind === "unchanged" ||
    (!draftHasExplicitKey && nonIdentityBlockers.length === 0);
  const dirty = hostNameChange !== undefined || userChange !== undefined || portChange !== undefined ||
    identityFileChange !== undefined || changesPassword || hasKeyPassphraseDraft || changesTOTP;
  const vaultOpen = vault?.unlocked === true && credentialOptionsStatus === "ready";
  const passwordResourcesReady = vaultOpen && eligibility !== null;
  const keyPassphraseResourcesReady = vaultOpen && keyOptionsStatus === "ready";
  const totpResourcesReady = vaultOpen;
  const vaultAllowsConfig = vault === null || vault.unlocked;
  const valid = hostError === "" && userError === "" && portError === "" && passwordAllowed && keyPassphraseValid &&
    (!changesPassword || passwordResourcesReady) &&
    (!hasKeyPassphraseDraft || keyPassphraseResourcesReady) &&
    (!changesTOTP || totpResourcesReady);
  const needsVault = (changesPassword && !passwordResourcesReady) ||
    (hasKeyPassphraseDraft && !keyPassphraseResourcesReady) ||
    (changesTOTP && !totpResourcesReady);
  const minimum = vault?.minPassphraseLength ?? 12;
  const canOpenVault = vault !== null && draft.masterPassword.length >= minimum &&
    (vault.exists || draft.masterConfirmation === draft.masterPassword);

  function request(): UpdateConnectionRequest {
    const built: UpdateConnectionRequest = {
      identity: detail.form.entry.identity,
      base: detail.file.contents,
      password: passwordChange,
      keyPassphrase: keyPassphraseChange,
      totp: totpChange,
    };
    if (hostNameChange !== undefined) built.hostName = hostNameChange;
    if (userChange !== undefined) built.user = userChange;
    if (portChange !== undefined) built.port = portChange;
    if (identityFileChange !== undefined) built.identityFile = identityFileChange;
    return built;
  }

  return {
    savedCredential,
    savedTOTP,
    identityFileChange,
    selectedPrivateKey,
    draftHasExplicitKey,
    passwordCleanup,
    confirmPasswordRoute,
    confirmTOTPRoute,
    namedKeyPassphrase,
    dedicatedKeyPassphrase,
    keyPassphraseStorageState,
    otherNamedKeyUses,
    hostError,
    userError,
    portError,
    passwordChange,
    keyPassphraseChange,
    changesPassword,
    changesTOTP,
    hasKeyPassphraseDraft,
    keyPassphraseValid,
    passwordBlockers: nonIdentityBlockers,
    passwordWarnings,
    passwordAllowed,
    dirty,
    valid,
    needsVault,
    vaultAllowsConfig,
    canOpenVault,
    request,
  };
}

export type BasicFormDerived = ReturnType<typeof deriveBasicForm>;
