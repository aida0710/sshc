import { ApiError, apiClient } from "./client";
import {
  asRecord,
  asArray,
  asString,
  asNumber,
  jsonHeaders,
  issueAction,
} from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type ConfigCheckResponse = components["schemas"]["ConfigCheckResponse"];
export type EffectiveResponse = components["schemas"]["EffectiveResponse"];
export type ReachabilityResponse =
  components["schemas"]["ReachabilityResponse"];
export type AuthenticationResponse =
  components["schemas"]["AuthenticationResponse"];
export type TerminalSettings = components["schemas"]["TerminalSettings"];
export type LocalShellProfile = components["schemas"]["LocalShellProfile"];
export type LocalShellProfileList =
  components["schemas"]["LocalShellProfileList"];
export type EngineSettings = components["schemas"]["EngineSettings"];
export type TerminalForward = components["schemas"]["TerminalForward"];
export type TerminalSession = components["schemas"]["TerminalSession"];
export type TerminalAppearance = components["schemas"]["TerminalAppearance"];
export type TerminalBackground = components["schemas"]["TerminalBackground"];
export type TerminalBackgroundList =
  components["schemas"]["TerminalBackgroundList"];
export type TerminalSessionList = components["schemas"]["TerminalSessionList"];
export type OpenTerminalSessionRequest =
  components["schemas"]["OpenTerminalSessionRequest"];
export type OpenTerminalSessionResponse =
  components["schemas"]["OpenTerminalSessionResponse"];
export type StartTerminalForwardRequest =
  components["schemas"]["StartTerminalForwardRequest"];
export type ResumeTerminalAgentRequest =
  components["schemas"]["ResumeTerminalAgentRequest"];
export type TerminalStreamTicket =
  components["schemas"]["TerminalStreamTicket"];
export type RecentConnection = components["schemas"]["RecentConnection"];
export type RecentConnectionList =
  components["schemas"]["RecentConnectionList"];
export type KnownHostsResponse = components["schemas"]["KnownHostsResponse"];
export type KnownHostEntry = components["schemas"]["KnownHostEntry"];
export type KnownHostsChangeResponse =
  components["schemas"]["KnownHostsChangeResponse"];
export type KnownHostsScanResponse =
  components["schemas"]["KnownHostsScanResponse"];
export type KnownHostCandidate = components["schemas"]["KnownHostCandidate"];
export type IssueActionResponse = components["schemas"]["IssueActionResponse"];
export type ChangeMasterPasswordResult =
  components["schemas"]["ChangeMasterPasswordResult"];
export type UpdateStatus = components["schemas"]["UpdateStatus"];
export type PasswordVaultStatus = components["schemas"]["PasswordVaultStatus"];
export type PasswordEligibility = components["schemas"]["PasswordEligibility"];
export type Credential = components["schemas"]["Credential"];
export type CredentialList = components["schemas"]["CredentialList"];
export type RevealCredentialResponse =
  components["schemas"]["RevealCredentialResponse"];
export type CredentialKind = "password" | "key_passphrase" | "totp";
export type SyncStatus = components["schemas"]["SyncStatus"];
export type SyncKeyResponse = components["schemas"]["SyncKeyResponse"];
export type SyncSettingsRequest = components["schemas"]["SyncSettingsRequest"];
export type SyncSetupCheckRequest =
  components["schemas"]["SyncSetupCheckRequest"];
export type SyncSetupCheckResponse =
  components["schemas"]["SyncSetupCheckResponse"];
export type SyncSetupRequest = components["schemas"]["SyncSetupRequest"];
export type SyncSetupResponse = components["schemas"]["SyncSetupResponse"];
export type SyncDirection = components["schemas"]["SyncDirection"];
export type SnapshotSummary = components["schemas"]["SnapshotSummary"];
export type SyncOperation = components["schemas"]["SyncOperation"];
export type PushResult = components["schemas"]["PushResult"];
export type PushResponse = components["schemas"]["PushResponse"];
export type SyncPushDraft = components["schemas"]["SyncPushDraft"];
export type SyncExclusions = components["schemas"]["SyncExclusions"];
export type PullResponse = components["schemas"]["PullResponse"];
export type SyncBucketStatus = components["schemas"]["SyncBucketStatus"];
export type SyncHistory = components["schemas"]["SyncHistory"];
export type SyncHistoryRevision = components["schemas"]["SyncHistoryRevision"];

const locallyExplainedSyncFailures = [
  "bucket_authentication_failed",
  "bucket_access_denied",
  "bucket_rate_limited",
  "bucket_unavailable",
  "bucket_refused",
  "bucket_timeout",
  "bucket_dns_failed",
  "bucket_tls_failed",
  "bucket_unreachable",
  "snapshot_download_incomplete",
  "snapshot_cost_refused",
  "snapshot_schema_unsupported",
  "snapshot_rejected",
  "snapshot_too_large",
  "wrong_passphrase",
  "sync_ignore_invalid",
] as const;
export type SyncHistoryDiff = components["schemas"]["SyncHistoryDiff"];

export const REACHABILITY_ACTION_KIND = "diagnostics.reachability";
export const AUTHENTICATION_ACTION_KIND = "diagnostics.authentication";
export const KNOWN_HOSTS_DELETE_ACTION_KIND = "known_hosts.delete";
export const KNOWN_HOSTS_SCAN_ACTION_KIND = "known_hosts.scan";
export const KNOWN_HOSTS_ADD_ACTION_KIND = "known_hosts.add";
export const SYNC_FORCE_PUSH_ACTION_KIND = "sync.force_push";
export const SYNC_FORCE_PUSH_TARGET = "remote-workspace";
export const CREDENTIAL_REVEAL_ACTION_KIND = "credential.reveal";

export type KnownHostAddition = Pick<
  KnownHostCandidate,
  "host" | "port" | "keyType" | "key"
>;

export type IntegrationsApi = {
  configCheck(): Promise<ConfigCheckResponse>;
  effective(alias: string): Promise<EffectiveResponse>;
  reachability(alias: string): Promise<ReachabilityResponse>;
  authentication(
    alias: string,
    acknowledgeExecutable: boolean,
  ): Promise<AuthenticationResponse>;
  terminalSessions(): Promise<TerminalSessionList>;
  recentConnections(): Promise<RecentConnectionList>;
  openTerminalSession(
    request: OpenTerminalSessionRequest,
  ): Promise<OpenTerminalSessionResponse>;
  terminalStreamTicket(id: string): Promise<TerminalStreamTicket>;
  reconnectTerminalSession(id: string): Promise<TerminalSessionList>;
  startTerminalForward(
    id: string,
    request: StartTerminalForwardRequest,
  ): Promise<TerminalSessionList>;
  stopTerminalForward(
    id: string,
    forwardId: string,
  ): Promise<TerminalSessionList>;
  resumeTerminalAgent?(
    id: string,
    request: ResumeTerminalAgentRequest,
  ): Promise<OpenTerminalSessionResponse>;
  renameTerminalSession(
    id: string,
    title: string | null,
  ): Promise<TerminalSessionList>;
  closeTerminalSession(id: string): Promise<TerminalSessionList>;
  knownHosts(query: string): Promise<KnownHostsResponse>;
  deleteKnownHosts(
    entries: { line: number; digest: string }[],
    path: string,
  ): Promise<KnownHostsChangeResponse>;
  scanKnownHosts(host: string, port: number): Promise<KnownHostsScanResponse>;
  addKnownHost(
    candidate: KnownHostAddition,
    expectedFingerprint: string,
    acknowledged: boolean,
  ): Promise<KnownHostsChangeResponse>;
  passwordVault(): Promise<PasswordVaultStatus>;
  initialiseVault(passphrase: string): Promise<PasswordVaultStatus>;
  unlockVault(passphrase: string): Promise<PasswordVaultStatus>;
  recoverCompatibleVault(passphrase: string): Promise<PasswordVaultStatus>;
  resetUnsupportedVault(passphrase: string): Promise<PasswordVaultStatus>;
  lockVault(): Promise<PasswordVaultStatus>;
  changeMasterPassword(
    current: string,
    next: string,
  ): Promise<ChangeMasterPasswordResult>;
  updateStatus(): Promise<UpdateStatus>;
  terminalSettings(): Promise<TerminalSettings>;
  localShellProfiles?(): Promise<LocalShellProfileList>;
  engineSettings(): Promise<EngineSettings>;
  setEngineSettings(settings: EngineSettings): Promise<void>;
  terminalBackgrounds(): Promise<TerminalBackgroundList>;
  addTerminalBackground(
    suggested: string,
    image: Blob,
  ): Promise<TerminalBackground>;
  deleteTerminalBackground(name: string): Promise<void>;
  setTerminalSettings(settings: TerminalSettings): Promise<void>;
  passwordEligibility(alias: string): Promise<PasswordEligibility>;
  credentials(): Promise<CredentialList>;
  storeCredential(
    kind: CredentialKind,
    name: string,
    secret: string,
  ): Promise<CredentialList>;
  revealCredential(
    kind: CredentialKind,
    name: string,
  ): Promise<RevealCredentialResponse>;
  updateCredential(
    kind: CredentialKind,
    currentName: string,
    name: string,
    secret: string,
  ): Promise<CredentialList>;
  deleteCredential(kind: CredentialKind, name: string): Promise<CredentialList>;
  assignCredential(
    kind: CredentialKind,
    subject: string,
    name: string,
  ): Promise<CredentialList>;
  unassignCredential(
    kind: CredentialKind,
    subject: string,
  ): Promise<CredentialList>;
  storePassword(alias: string, password: string): Promise<PasswordVaultStatus>;
  forgetPassword(alias: string): Promise<PasswordVaultStatus>;
  syncStatus(): Promise<SyncStatus>;
  checkSyncSetup(
    settings: SyncSetupCheckRequest,
  ): Promise<SyncSetupCheckResponse>;
  completeSyncSetup(settings: SyncSetupRequest): Promise<SyncSetupResponse>;
  configureSync(settings: SyncSettingsRequest): Promise<SyncStatus>;
  syncExclusions(): Promise<SyncExclusions>;
  saveSyncExclusions(document: string): Promise<SyncExclusions>;
  syncPushDraft(): Promise<SyncPushDraft>;
  pushSnapshot(message: string): Promise<PushResponse>;
  forcePushSnapshot(message: string): Promise<PushResponse>;
  syncBucketStatus(): Promise<SyncBucketStatus>;
  syncHistory(): Promise<SyncHistory>;
  diffSyncHistory(key: string): Promise<SyncHistoryDiff>;
  pullSnapshot(
    apply: boolean,
    resolve?: "local" | "remote",
    historyKey?: string,
    expected?: Pick<PullResponse, "remoteETag" | "remoteRevision">,
    acceptRemoteHead?: boolean,
  ): Promise<PullResponse>;
  setSyncKey(
    key?: string,
    confirmHistoryLoss?: boolean,
  ): Promise<SyncKeyResponse>;
  setAutoSync(enabled: boolean): Promise<SyncStatus>;
  syncNow(): Promise<SyncStatus>;
};

function validateUpdate(value: unknown): UpdateStatus {
  return validateOpenAPISchema<UpdateStatus>("UpdateStatus", value);
}

function validateConfigCheck(value: unknown): ConfigCheckResponse {
  return validateOpenAPISchema<ConfigCheckResponse>("ConfigCheckResponse", value);
}

function validateEffective(value: unknown): EffectiveResponse {
  return validateOpenAPISchema<EffectiveResponse>("EffectiveResponse", value);
}

function validateReachability(value: unknown): ReachabilityResponse {
  return validateOpenAPISchema<ReachabilityResponse>("ReachabilityResponse", value);
}

function validateAuthentication(value: unknown): AuthenticationResponse {
  return validateOpenAPISchema<AuthenticationResponse>("AuthenticationResponse", value);
}

function validateTerminalSessionList(value: unknown): TerminalSessionList {
  return validateOpenAPISchema<TerminalSessionList>("TerminalSessionList", value);
}

function validateRecentConnections(value: unknown): RecentConnectionList {
  return validateOpenAPISchema<RecentConnectionList>("RecentConnectionList", value);
}

function validateOpenTerminalSession(
  value: unknown,
): OpenTerminalSessionResponse {
  return validateOpenAPISchema<OpenTerminalSessionResponse>("OpenTerminalSessionResponse", value);
}

function validateLocalShellProfiles(value: unknown): LocalShellProfileList {
  return validateOpenAPISchema<LocalShellProfileList>("LocalShellProfileList", value);
}

function validateStreamTicket(value: unknown): TerminalStreamTicket {
  return validateOpenAPISchema<TerminalStreamTicket>("TerminalStreamTicket", value);
}

function validateKnownHosts(value: unknown): KnownHostsResponse {
  return validateOpenAPISchema<KnownHostsResponse>("KnownHostsResponse", value);
}

function validateChange(value: unknown): KnownHostsChangeResponse {
  return validateOpenAPISchema<KnownHostsChangeResponse>("KnownHostsChangeResponse", value);
}

function validateScan(value: unknown): KnownHostsScanResponse {
  return validateOpenAPISchema<KnownHostsScanResponse>("KnownHostsScanResponse", value);
}

async function postJSON<T>(
  path: string,
  body: unknown,
  actionToken?: string,
  locallyHandledCodes?: readonly string[],
): Promise<T> {
  const headers: Record<string, string> = { ...jsonHeaders };
  if (actionToken) headers["X-SSHC-Action"] = actionToken;
  return apiClient.mutate<T>(
    path,
    { method: "POST", headers, body: JSON.stringify(body) },
    locallyHandledCodes === undefined ? {} : { locallyHandledCodes },
  );
}

async function postEmpty<T>(path: string): Promise<T> {
  return apiClient.mutate<T>(path, { method: "POST" });
}

function validateVaultStatus(value: unknown): PasswordVaultStatus {
  return validateOpenAPISchema<PasswordVaultStatus>("PasswordVaultStatus", value);
}

function credentialPath(kind: CredentialKind, name: string): string {
  return `/api/v1/credentials/${kind}/${encodeURIComponent(name)}`;
}

function validateCredentialList(value: unknown): CredentialList {
  return validateOpenAPISchema<CredentialList>("CredentialList", value);
}

function validateRevealCredential(value: unknown): RevealCredentialResponse {
  return validateOpenAPISchema<RevealCredentialResponse>("RevealCredentialResponse", value);
}

function validatePasswordEligibility(value: unknown): PasswordEligibility {
  return validateOpenAPISchema<PasswordEligibility>("PasswordEligibility", value);
}

function validateSyncStatus(value: unknown): SyncStatus {
  return validateOpenAPISchema<SyncStatus>("SyncStatus", value);
}

function validateSyncSetupCheck(value: unknown): SyncSetupCheckResponse {
  return validateOpenAPISchema<SyncSetupCheckResponse>("SyncSetupCheckResponse", value);
}

function validateSyncSetup(value: unknown): SyncSetupResponse {
  return validateOpenAPISchema<SyncSetupResponse>("SyncSetupResponse", value);
}

function validateSyncKey(value: unknown): SyncKeyResponse {
  return validateOpenAPISchema<SyncKeyResponse>("SyncKeyResponse", value);
}

function validatePushResponse(value: unknown): PushResponse {
  return validateOpenAPISchema<PushResponse>("PushResponse", value);
}

function validateSyncPushDraft(value: unknown): SyncPushDraft {
  return validateOpenAPISchema<SyncPushDraft>("SyncPushDraft", value);
}

function validateSyncExclusions(value: unknown): SyncExclusions {
  return validateOpenAPISchema<SyncExclusions>("SyncExclusions", value);
}

function validateSyncBucketStatus(value: unknown): SyncBucketStatus {
  return validateOpenAPISchema<SyncBucketStatus>("SyncBucketStatus", value);
}

function validateSyncHistory(value: unknown): SyncHistory {
  return validateOpenAPISchema<SyncHistory>("SyncHistory", value);
}

function validateSyncHistoryDiff(value: unknown): SyncHistoryDiff {
  return validateOpenAPISchema<SyncHistoryDiff>("SyncHistoryDiff", value);
}

function validatePullResponse(value: unknown): PullResponse {
  return validateOpenAPISchema<PullResponse>("PullResponse", value);
}

function readAppearance(value: unknown): TerminalAppearance {
  const record = asRecord(value);
  return {
    ...(typeof record.palette === "string" ? { palette: record.palette } : {}),
    ...(typeof record.font === "string" ? { font: record.font } : {}),
    ...(typeof record.background === "string"
      ? { background: record.background }
      : {}),
    ...(typeof record.backgroundTint === "number"
      ? { backgroundTint: record.backgroundTint }
      : {}),
  };
}

function validateBackground(value: unknown): TerminalBackground {
  return validateOpenAPISchema<TerminalBackground>("TerminalBackground", value);
}

export const integrationsApi: IntegrationsApi = {
  async configCheck() {
    return validateConfigCheck(
      await postEmpty<unknown>("/api/v1/diagnostics/config"),
    );
  },
  async effective(alias) {
    return validateEffective(
      await postJSON<unknown>("/api/v1/diagnostics/effective", { alias }),
    );
  },
  async reachability(alias) {
    const token = await issueAction(REACHABILITY_ACTION_KIND, alias);
    return validateReachability(
      await postJSON<unknown>(
        "/api/v1/diagnostics/reachability",
        { alias },
        token,
      ),
    );
  },
  async authentication(alias, acknowledgeExecutable) {
    const token = await issueAction(AUTHENTICATION_ACTION_KIND, alias);
    return validateAuthentication(
      await postJSON<unknown>(
        "/api/v1/diagnostics/authentication",
        { alias, acknowledgeExecutable },
        token,
      ),
    );
  },
  async terminalSessions() {
    return validateTerminalSessionList(
      await apiClient.read("/api/v1/terminal/sessions"),
    );
  },
  async recentConnections() {
    return validateRecentConnections(
      await apiClient.read("/api/v1/connections/recent"),
    );
  },
  async openTerminalSession(request) {
    return validateOpenTerminalSession(
      await postJSON<unknown>("/api/v1/terminal/sessions", request),
    );
  },
  async terminalStreamTicket(id) {
    return validateStreamTicket(
      await postEmpty<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/stream`,
      ),
    );
  },
  async reconnectTerminalSession(id) {
    return validateTerminalSessionList(
      await postEmpty<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/reconnect`,
      ),
    );
  },
  async startTerminalForward(id, request) {
    return validateTerminalSessionList(
      await postJSON<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/forwards`,
        request,
      ),
    );
  },
  async stopTerminalForward(id, forwardId) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/forwards/${encodeURIComponent(forwardId)}`,
        {
          method: "DELETE",
        },
      ),
    );
  },
  async resumeTerminalAgent(id, request) {
    return validateOpenTerminalSession(
      await postJSON<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/agent/resume`,
        request,
      ),
    );
  },
  async renameTerminalSession(id, title) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/title`,
        {
          method: "PUT",
          headers: { ...jsonHeaders },
          body: JSON.stringify({ title }),
        },
      ),
    );
  },
  async closeTerminalSession(id) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}`,
        {
          method: "DELETE",
        },
      ),
    );
  },
  async passwordVault() {
    return validateVaultStatus(await apiClient.read("/api/v1/passwords"));
  },
  async initialiseVault(passphrase) {
    return validateVaultStatus(
      await postJSON<unknown>("/api/v1/passwords/initialise", { passphrase }),
    );
  },
  async unlockVault(passphrase) {
    return validateVaultStatus(
      await postJSON<unknown>("/api/v1/passwords/unlock", { passphrase }),
    );
  },
  async recoverCompatibleVault(passphrase) {
    return validateVaultStatus(
      await postJSON<unknown>("/api/v1/passwords/recover-compatible-backup", {
        passphrase,
      }),
    );
  },
  async resetUnsupportedVault(passphrase) {
    return validateVaultStatus(
      await postJSON<unknown>("/api/v1/passwords/reset-unsupported", {
        passphrase,
        acknowledged: true,
      }),
    );
  },
  async lockVault() {
    return validateVaultStatus(
      await postEmpty<unknown>("/api/v1/passwords/lock"),
    );
  },
  async updateStatus() {
    return validateUpdate(await apiClient.read("/api/v1/update"));
  },
  async terminalSettings() {
    const metadata = asRecord(await apiClient.read("/api/v1/metadata"));
    if (metadata.embeddedTerminal === undefined) return {};
    const terminal = asRecord(metadata.embeddedTerminal);
    return {
      ...(typeof terminal.startDirectory === "string" &&
      terminal.startDirectory !== ""
        ? { startDirectory: terminal.startDirectory }
        : {}),
      ...(typeof terminal.maxSessions === "number"
        ? { maxSessions: terminal.maxSessions }
        : {}),
      ...(typeof terminal.scrollbackBytes === "number"
        ? { scrollbackBytes: terminal.scrollbackBytes }
        : {}),
      ...(typeof terminal.browserScrollbackLines === "number" &&
      terminal.browserScrollbackLines >= 1000 &&
      terminal.browserScrollbackLines <= 100000
        ? { browserScrollbackLines: terminal.browserScrollbackLines }
        : {}),
      ...(typeof terminal.fontSize === "number"
        ? { fontSize: terminal.fontSize }
        : {}),
      ...(typeof terminal.verbosity === "number"
        ? { verbosity: terminal.verbosity }
        : {}),
      ...(typeof terminal.reconnect === "number"
        ? { reconnect: terminal.reconnect }
        : {}),
      ...(typeof terminal.copyOnSelect === "boolean"
        ? { copyOnSelect: terminal.copyOnSelect }
        : {}),
      ...(typeof terminal.rightClickPaste === "boolean"
        ? { rightClickPaste: terminal.rightClickPaste }
        : {}),
      ...(typeof terminal.webgl === "boolean" ? { webgl: terminal.webgl } : {}),
      ...(typeof terminal.osc52 === "boolean" ? { osc52: terminal.osc52 } : {}),
      ...(typeof terminal.jisYenBackslash === "boolean"
        ? { jisYenBackslash: terminal.jisYenBackslash }
        : {}),
      ...(typeof terminal.localShellProfile === "string" &&
      /^[a-z0-9-]{1,64}$/.test(terminal.localShellProfile)
        ? { localShellProfile: terminal.localShellProfile }
        : {}),
      ...(terminal.appearance === undefined
        ? {}
        : { appearance: readAppearance(terminal.appearance) }),
    };
  },
  async localShellProfiles() {
    return validateLocalShellProfiles(
      await apiClient.read("/api/v1/terminal/shell-profiles"),
    );
  },
  async engineSettings() {
    const metadata = asRecord(await apiClient.read("/api/v1/metadata"));
    if (metadata.engine === undefined) return {};
    const engine = asRecord(metadata.engine);
    const settings: EngineSettings = typeof engine.port === "number" ? { port: engine.port } : {};
    if (engine.vaultAutoLock !== undefined) {
      const autoLock = asRecord(engine.vaultAutoLock);
      const mode = asString(autoLock.mode);
      if (mode === "restart") {
        settings.vaultAutoLock = { mode };
      } else if (mode === "idle") {
        const value = asNumber(autoLock.value);
        const unit = asString(autoLock.unit);
        if (Number.isSafeInteger(value) && value >= 1 && value <= 999 &&
            (unit === "minutes" || unit === "hours")) {
          settings.vaultAutoLock = { mode, value, unit };
        }
      }
    }
    return settings;
  },
  async setEngineSettings(settings) {
    await apiClient.mutate("/api/v1/metadata/engine", {
      method: "PUT",
      headers: jsonHeaders,
      body: JSON.stringify(settings),
    });
  },
  async terminalBackgrounds() {
    const record = asRecord(
      await apiClient.read("/api/v1/terminal/backgrounds"),
    );
    return {
      backgrounds: asArray(record.backgrounds).map(validateBackground),
      remainingBytes: asNumber(record.remainingBytes),
    };
  },
  async addTerminalBackground(suggested, image) {
    return validateBackground(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/backgrounds?name=${encodeURIComponent(suggested)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/octet-stream" },
          body: image,
        },
      ),
    );
  },
  async deleteTerminalBackground(name) {
    const response = await apiClient.send(
      `/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`,
      {
        method: "DELETE",
      },
    );
    if (!response.ok)
      throw new ApiError("background_not_removed", response.status, null);
  },
  async setTerminalSettings(settings) {
    await apiClient.mutate("/api/v1/metadata/terminal", {
      method: "PUT",
      headers: jsonHeaders,
      body: JSON.stringify(settings),
    });
  },
  async changeMasterPassword(current, next) {
    const answer = await postJSON<unknown>("/api/v1/passwords/change", {
      current,
      next,
    });
    const record = asRecord(answer);
    return { vault: validateVaultStatus(record.vault) };
  },
  async passwordEligibility(alias) {
    return validatePasswordEligibility(
      await apiClient.read(
        `/api/v1/passwords/${encodeURIComponent(alias)}/eligibility`,
      ),
    );
  },
  async storePassword(alias, password) {
    return validateVaultStatus(
      await apiClient.mutate<unknown>(
        `/api/v1/passwords/${encodeURIComponent(alias)}`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ password }),
        },
      ),
    );
  },
  async forgetPassword(alias) {
    return validateVaultStatus(
      await apiClient.mutate<unknown>(
        `/api/v1/passwords/${encodeURIComponent(alias)}`,
        { method: "DELETE" },
      ),
    );
  },
  async credentials() {
    return validateCredentialList(await apiClient.read("/api/v1/credentials"));
  },
  async storeCredential(kind, name, secret) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, name), {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ secret }),
      }),
    );
  },
  async revealCredential(kind, name) {
    const token = await issueAction(
      CREDENTIAL_REVEAL_ACTION_KIND,
      `${kind}\n${name}`,
    );
    return validateRevealCredential(
      await apiClient.mutate<unknown>(`${credentialPath(kind, name)}/reveal`, {
        method: "POST",
        headers: { "X-SSHC-Action": token },
      }),
    );
  },
  async updateCredential(kind, currentName, name, secret) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, currentName), {
        method: "PATCH",
        headers: jsonHeaders,
        body: JSON.stringify({ name, secret }),
      }),
    );
  },
  async deleteCredential(kind, name) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, name), {
        method: "DELETE",
      }),
    );
  },
  async assignCredential(kind, subject, name) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(`/api/v1/credentials/${kind}/assign`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ subject, name }),
      }),
    );
  },
  async unassignCredential(kind, subject) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(
        `/api/v1/credentials/${kind}/assign/${encodeURIComponent(subject)}`,
        { method: "DELETE" },
      ),
    );
  },
  async syncStatus() {
    return validateSyncStatus(await apiClient.read("/api/v1/sync"));
  },
  async checkSyncSetup(settings) {
    return validateSyncSetupCheck(
      await apiClient.mutate<unknown>("/api/v1/sync/setup/check", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }, { locallyHandledCodes: locallyExplainedSyncFailures }),
    );
  },
  async completeSyncSetup(settings) {
    return validateSyncSetup(
      await apiClient.mutate<unknown>("/api/v1/sync/setup", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }, { locallyHandledCodes: locallyExplainedSyncFailures }),
    );
  },
  async configureSync(settings) {
    return validateSyncStatus(
      await apiClient.mutate<unknown>("/api/v1/sync/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }),
    );
  },
  async syncExclusions() {
    return validateSyncExclusions(
      await apiClient.read("/api/v1/sync/exclusions"),
    );
  },
  async saveSyncExclusions(document) {
    return validateSyncExclusions(
      await apiClient.mutate<unknown>("/api/v1/sync/exclusions", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ document }),
      }, { locallyHandledCodes: ["sync_ignore_invalid"] }),
    );
  },
  async syncPushDraft() {
    return validateSyncPushDraft(await apiClient.read("/api/v1/sync/push"));
  },
  async pushSnapshot(message) {
    return validatePushResponse(
      await postJSON<unknown>("/api/v1/sync/push", { message }),
    );
  },
  async forcePushSnapshot(message) {
    const token = await issueAction(
      SYNC_FORCE_PUSH_ACTION_KIND,
      SYNC_FORCE_PUSH_TARGET,
    );
    return validatePushResponse(
      await postJSON<unknown>("/api/v1/sync/force-push", { message }, token),
    );
  },
  async syncBucketStatus() {
    return validateSyncBucketStatus(
      await apiClient.read("/api/v1/sync/bucket"),
    );
  },
  async syncHistory() {
    return validateSyncHistory(await apiClient.read("/api/v1/sync/history"));
  },
  async diffSyncHistory(key) {
    return validateSyncHistoryDiff(
      await postJSON<unknown>("/api/v1/sync/history/diff", { key }),
    );
  },
  async pullSnapshot(apply, resolve, historyKey, expected, acceptRemoteHead) {
    if (apply && expected === undefined) throw new Error("invalid_request");
    const request = {
      apply,
      ...(resolve === undefined ? {} : { resolve }),
      ...(historyKey === undefined ? {} : { historyKey }),
      ...(acceptRemoteHead === undefined ? {} : { acceptRemoteHead }),
      ...(expected === undefined
        ? {}
        : {
            expectedETag: expected.remoteETag,
            expectedRevision: expected.remoteRevision,
          }),
    };
    return validatePullResponse(
      await postJSON<unknown>("/api/v1/sync/pull", request, undefined, [
        ...locallyExplainedSyncFailures,
        "sync_failed",
        "sync_local_changed",
        "sync_workspace_busy",
      ]),
    );
  },
  async setSyncKey(key, confirmHistoryLoss) {
    return validateSyncKey(
      await apiClient.mutate<unknown>("/api/v1/sync/key", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...(key === undefined ? {} : { key }),
          ...(confirmHistoryLoss === undefined ? {} : { confirmHistoryLoss }),
        }),
      }),
    );
  },
  async setAutoSync(enabled) {
    return validateSyncStatus(
      await apiClient.mutate<unknown>("/api/v1/sync/auto", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled }),
      }),
    );
  },
  async syncNow() {
    return validateSyncStatus(await postEmpty<unknown>("/api/v1/sync/now"));
  },
  async knownHosts(query) {
    return validateKnownHosts(
      await apiClient.read(
        `/api/v1/known-hosts?query=${encodeURIComponent(query)}`,
      ),
    );
  },
  async deleteKnownHosts(entries, path) {
    const token = await issueAction(KNOWN_HOSTS_DELETE_ACTION_KIND, path);
    return validateChange(
      await postJSON<unknown>("/api/v1/known-hosts/delete", { entries }, token),
    );
  },
  async scanKnownHosts(host, port) {
    const token = await issueAction(KNOWN_HOSTS_SCAN_ACTION_KIND, host);
    return validateScan(
      await postJSON<unknown>(
        "/api/v1/known-hosts/scan",
        { host, port },
        token,
      ),
    );
  },
  async addKnownHost(candidate, expectedFingerprint, acknowledged) {
    const token = await issueAction(
      KNOWN_HOSTS_ADD_ACTION_KIND,
      candidate.host,
    );
    return validateChange(
      await postJSON<unknown>(
        "/api/v1/known-hosts/add",
        {
          host: candidate.host,
          port: candidate.port,
          keyType: candidate.keyType,
          key: candidate.key,
          expectedFingerprint,
          acknowledged,
        },
        token,
      ),
    );
  },
};
