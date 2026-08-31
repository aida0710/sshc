import { ApiError, apiClient } from "./client";
import {
  asRecord,
  asArray,
  asString,
  asNumber,
  asBoolean,
  jsonHeaders,
  issueAction,
} from "./guards";
import type { components } from "./schema";

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
export type CredentialKind = "password" | "key_passphrase";
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
  const record = asRecord(value);
  if (
    typeof record.current !== "string" ||
    typeof record.available !== "boolean"
  ) {
    throw new Error("invalid_response");
  }
  return {
    current: record.current,
    available: record.available,
    ...(typeof record.latest === "string" ? { latest: record.latest } : {}),
    ...(typeof record.pageUrl === "string" ? { pageUrl: record.pageUrl } : {}),
  };
}

function asNonnegativeInteger(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new Error("invalid_response");
  }
  return value;
}

function validateConfigCheck(value: unknown): ConfigCheckResponse {
  const record = asRecord(value);
  asString(record.root);
  for (const file of asArray(record.files)) {
    const entry = asRecord(file);
    asString(entry.path);
    asBoolean(entry.editable);
    asBoolean(entry.missing);
    asNumber(entry.loads);
    asNumber(entry.includes);
  }
  for (const diagnostic of asArray(record.diagnostics)) {
    const entry = asRecord(diagnostic);
    asString(entry.severity);
    asString(entry.code);
    asString(entry.path);
    asNumber(entry.line);
    asString(entry.detail);
  }
  return record as unknown as ConfigCheckResponse;
}

function validateEffective(value: unknown): EffectiveResponse {
  const record = asRecord(value);
  asString(record.alias);
  asString(record.tokenWarning);
  for (const directive of asArray(record.executableDirectives)) {
    const entry = asRecord(directive);
    asString(entry.keyword);
    asString(entry.command);
    asString(entry.path);
    asNumber(entry.line);
    asBoolean(entry.onEvaluate);
    asBoolean(entry.onConnect);
    asBoolean(entry.overridable);
  }
  for (const source of asArray(record.sources)) {
    const entry = asRecord(source);
    asString(entry.keyword);
    asString(entry.value);
    asString(entry.path);
    asNumber(entry.line);
    asBoolean(entry.winner);
  }
  for (const note of asArray(record.complexities)) {
    const entry = asRecord(note);
    asString(entry.code);
    asString(entry.path);
    asNumber(entry.line);
    asString(entry.condition);
    asString(entry.detail);
  }
  for (const stage of asArray(record.route)) {
    const entry = asRecord(stage);
    asNumber(entry.order);
    asNumber(entry.depth);
    asString(entry.parent);
    asString(entry.hop);
    asString(entry.hostname);
    asString(entry.user);
    asString(entry.port);
    asBoolean(entry.complex);
  }
  return record as unknown as EffectiveResponse;
}

function validateReachability(value: unknown): ReachabilityResponse {
  const record = asRecord(value);
  asString(record.address);
  asString(record.outcome);
  asNumber(record.elapsedMs);
  asString(record.detail);
  asString(record.notice);
  return record as unknown as ReachabilityResponse;
}

function validateAuthentication(value: unknown): AuthenticationResponse {
  const record = asRecord(value);
  asString(record.outcome);
  asBoolean(record.authenticated);
  asString(record.method);
  asString(record.detail);
  asBoolean(record.truncated);
  asNumber(record.elapsedMs);
  return record as unknown as AuthenticationResponse;
}

function validateTerminalSession(value: unknown): TerminalSession {
  const record = asRecord(value);
  asString(record.id);
  const kind = asString(record.kind);
  if (kind !== "ssh" && kind !== "shell") throw new Error("invalid_response");
  asString(record.title);
  asString(record.startedAt);
  const state = asString(record.state);
  if (
    state !== "connecting" &&
    state !== "connected" &&
    state !== "reconnecting" &&
    state !== "exited"
  ) {
    throw new Error("invalid_response");
  }
  asString(record.problem);
  if (record.progress !== undefined) {
    const progress = asRecord(record.progress);
    const phase = asString(progress.phase);
    if (
      ![
        "dialing",
        "host_key",
        "authenticating",
        "authenticated",
        "opening_session",
      ].includes(phase)
    ) {
      throw new Error("invalid_response");
    }
    asString(progress.alias);
    asString(progress.hostName);
    asString(progress.user);
    const hop = asNonnegativeInteger(progress.hop);
    const hops = asNonnegativeInteger(progress.hops);
    if (hop < 1 || hops < 1 || hop > hops) throw new Error("invalid_response");
  }
  if (record.reconnect !== undefined) {
    const reconnect = asRecord(record.reconnect);
    const attempt = asNonnegativeInteger(reconnect.attempt);
    const limit = asNonnegativeInteger(reconnect.limit);
    if (attempt < 1 || attempt > 5 || limit < 1 || limit > 5 || attempt > limit)
      throw new Error("invalid_response");
    asString(reconnect.retryAt);
    asString(reconnect.problem);
  }
  if (record.alias !== undefined) asString(record.alias);
  if (record.exited !== undefined) {
    const exited = asRecord(record.exited);
    asNumber(exited.code);
    asString(exited.signal);
    asString(exited.at);
  }
  if (record.forwards !== undefined) {
    for (const forward of asArray(record.forwards)) {
      const entry = asRecord(forward);
      asString(entry.id);
      const kind = asString(entry.kind);
      if (kind !== "local" && kind !== "dynamic" && kind !== "agent")
        throw new Error("invalid_response");
      asString(entry.listen);
      asString(entry.to);
      asString(entry.problem);
      asBoolean(entry.temporary);
    }
  }
  if (record.presentation !== undefined) {
    const presentation = asRecord(record.presentation);
    asString(presentation.displayTitle);
    const source = asString(presentation.titleSource);
    if (
      !["user", "agent", "candidate", "connection", "fallback"].includes(source)
    ) {
      throw new Error("invalid_response");
    }
    asBoolean(presentation.titlePinned);
  }
  if (record.agent !== undefined) {
    const agent = asRecord(record.agent);
    if (!["claude", "codex", "opencode"].includes(asString(agent.kind)))
      throw new Error("invalid_response");
    if (
      !["working", "attention", "ready", "unknown"].includes(
        asString(agent.state),
      )
    ) {
      throw new Error("invalid_response");
    }
    asBoolean(agent.resumable);
    asNonnegativeInteger(agent.observationVersion);
    asNonnegativeInteger(agent.signalVersion);
    if (agent.cwd !== undefined) asString(agent.cwd);
    if (agent.model !== undefined) asString(agent.model);
    if (agent.sessionName !== undefined) asString(agent.sessionName);
    if (agent.lastSignal !== undefined) {
      const signal = asRecord(agent.lastSignal);
      if (!["attention", "completed"].includes(asString(signal.kind)))
        throw new Error("invalid_response");
      asString(signal.occurredAt);
    }
  }
  return record as unknown as TerminalSession;
}

function validateTerminalSessionList(value: unknown): TerminalSessionList {
  const record = asRecord(value);
  for (const session of asArray(record.sessions))
    validateTerminalSession(session);
  asNonnegativeInteger(record.maxSessions);
  return record as unknown as TerminalSessionList;
}

function validateRecentConnections(value: unknown): RecentConnectionList {
  const record = asRecord(value);
  const connections = asArray(record.connections).map((value) => {
    const connection = asRecord(value);
    return {
      alias: asString(connection.alias),
      hostName: asString(connection.hostName),
      user: asString(connection.user),
      port: asString(connection.port),
      lastConnectedAt: asString(connection.lastConnectedAt),
    };
  });
  return { connections };
}

function validateOpenTerminalSession(
  value: unknown,
): OpenTerminalSessionResponse {
  const record = asRecord(value);
  validateTerminalSession(record.session);
  asString(record.streamTicket);
  return record as unknown as OpenTerminalSessionResponse;
}

function validateLocalShellProfiles(value: unknown): LocalShellProfileList {
  const record = asRecord(value);
  const profiles = asArray(record.profiles).map((value) => {
    const profile = asRecord(value);
    return {
      id: asString(profile.id),
      label: asString(profile.label),
      path: asString(profile.path),
      arguments: asArray(profile.arguments).map(asString),
      default: asBoolean(profile.default),
    };
  });
  return { profiles };
}

function validateStreamTicket(value: unknown): TerminalStreamTicket {
  const record = asRecord(value);
  asString(record.streamTicket);
  return record as unknown as TerminalStreamTicket;
}

function validateKnownHosts(value: unknown): KnownHostsResponse {
  const record = asRecord(value);
  asString(record.path);
  for (const entry of asArray(record.entries)) {
    const item = asRecord(entry);
    asNumber(item.line);
    asString(item.digest);
    asString(item.marker);
    asArray(item.hosts);
    asBoolean(item.hashed);
    asString(item.keyType);
    asString(item.fingerprint);
    asString(item.comment);
  }
  return record as unknown as KnownHostsResponse;
}

function validateChange(value: unknown): KnownHostsChangeResponse {
  const record = asRecord(value);
  asBoolean(record.changed);
  asString(record.transactionId);
  return record as unknown as KnownHostsChangeResponse;
}

function validateScan(value: unknown): KnownHostsScanResponse {
  const record = asRecord(value);
  asString(record.notice);
  for (const candidate of asArray(record.candidates)) {
    const item = asRecord(candidate);
    asString(item.host);
    asNumber(item.port);
    asString(item.keyType);
    asString(item.key);
    asString(item.fingerprint);
    asBoolean(item.verified);
  }
  return record as unknown as KnownHostsScanResponse;
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

function validateVaultStatus(value: unknown): PasswordVaultStatus {
  const record = asRecord(value);
  asBoolean(record.exists);
  asBoolean(record.unlocked);
  for (const alias of asArray(record.aliases)) asString(alias);
  for (const relativePath of asArray(record.dedicatedKeyPassphrases))
    asString(relativePath);
  if (record.migratedFromVersion !== undefined)
    asNumber(record.migratedFromVersion);
  if (record.migratedToVersion !== undefined)
    asNumber(record.migratedToVersion);
  return record as unknown as PasswordVaultStatus;
}

function credentialPath(kind: CredentialKind, name: string): string {
  return `/api/v1/credentials/${kind}/${encodeURIComponent(name)}`;
}

function validateCredentialList(value: unknown): CredentialList {
  const record = asRecord(value);
  for (const credential of asArray(record.credentials)) {
    const entry = asRecord(credential);
    asString(entry.kind);
    asString(entry.name);
    for (const use of asArray(entry.uses)) asString(use);
    for (const host of asArray(entry.hosts)) asString(host);
  }
  for (const dedicated of asArray(record.dedicatedKeyPassphrases)) {
    const entry = asRecord(dedicated);
    asString(entry.key);
    for (const host of asArray(entry.hosts)) asString(host);
  }
  asBoolean(record.keyHostUsageComplete);
  return record as unknown as CredentialList;
}

function validateRevealCredential(value: unknown): RevealCredentialResponse {
  const record = asRecord(value);
  asString(record.kind);
  asString(record.name);
  asString(record.secret);
  return record as unknown as RevealCredentialResponse;
}

function validatePasswordEligibility(value: unknown): PasswordEligibility {
  const record = asRecord(value);
  asString(record.alias);
  asBoolean(record.storable);
  for (const group of [record.blockers, record.warnings]) {
    for (const notice of asArray(group)) asString(asRecord(notice).code);
  }
  return record as unknown as PasswordEligibility;
}

function validateSyncStatus(value: unknown): SyncStatus {
  const record = asRecord(value);
  asBoolean(record.configured);
  asBoolean(record.locked);
  asBoolean(record.keyConfigured);
  const auto = asRecord(record.auto);
  asBoolean(auto.enabled);
  const phase = asString(auto.phase);
  if (
    phase !== "idle" &&
    phase !== "running" &&
    phase !== "blocked" &&
    phase !== "failed"
  ) {
    throw new Error(`unexpected auto sync phase: ${phase}`);
  }
  asString(record.endpoint);
  asString(record.bucket);
  asBoolean(record.synced);
  const direction = asString(record.direction);
  if (direction !== "both" && direction !== "push" && direction !== "pull") {
    throw new Error(`unexpected sync direction: ${direction}`);
  }
  if (record.path !== undefined) asString(record.path);
  if (record.region !== undefined) asString(record.region);
  if (record.lastSyncedAt !== undefined) asString(record.lastSyncedAt);
  if (record.origin !== undefined) asString(record.origin);
  if (record.fileCount !== undefined) asNonnegativeInteger(record.fileCount);
  if (record.lastOperation !== undefined)
    validateSyncOperation(record.lastOperation);
  return record as unknown as SyncStatus;
}

function validateSyncSetupCheck(value: unknown): SyncSetupCheckResponse {
  const record = asRecord(value);
  const state = asString(record.state);
  if (state !== "empty" && state !== "existing" && state !== "incomplete") {
    throw new Error(`unexpected sync setup state: ${state}`);
  }
  asBoolean(record.historyPresent);
  asString(record.checkedAt);
  if (record.etag !== undefined) asString(record.etag);
  return record as unknown as SyncSetupCheckResponse;
}

function validateSyncSetup(value: unknown): SyncSetupResponse {
  const record = asRecord(value);
  validateSyncStatus(record.status);
  if (record.generatedKey !== undefined) asString(record.generatedKey);
  return record as unknown as SyncSetupResponse;
}

function validateSnapshotSummary(value: unknown): SnapshotSummary {
  const record = asRecord(value);
  asString(record.createdAt);
  asNonnegativeInteger(record.fileCount);
  asNonnegativeInteger(record.sourceBytes);
  asNonnegativeInteger(record.snapshotBytes);
  return record as unknown as SnapshotSummary;
}

function validateSyncOperation(value: unknown): SyncOperation {
  const record = asRecord(value);
  const kind = asString(record.kind);
  validateSnapshotSummary(record.summary);
  asString(record.completedAt);
  if (kind === "push") {
    asNonnegativeInteger(record.objectCount);
    asNonnegativeInteger(record.uploadedBytes);
  } else if (kind === "apply") {
    asNonnegativeInteger(record.downloadedBytes);
    asNonnegativeInteger(record.written);
    asNonnegativeInteger(record.removed);
  } else {
    throw new Error("invalid_response");
  }
  return record as unknown as SyncOperation;
}

function validateSyncKey(value: unknown): SyncKeyResponse {
  const record = asRecord(value);
  asString(record.key);
  return record as unknown as SyncKeyResponse;
}

function validatePushResponse(value: unknown): PushResponse {
  const record = asRecord(value);
  validateSyncStatus(record.status);
  const result = asRecord(record.result);
  validateSnapshotSummary(result.summary);
  asNonnegativeInteger(result.objectCount);
  asNonnegativeInteger(result.uploadedBytes);
  asString(result.completedAt);
  return record as unknown as PushResponse;
}

function validateSyncPushDraft(value: unknown): SyncPushDraft {
  const record = asRecord(value);
  asString(record.message);
  asNonnegativeInteger(record.added);
  asNonnegativeInteger(record.modified);
  asNonnegativeInteger(record.removed);
  return record as unknown as SyncPushDraft;
}

function validateSyncExclusions(value: unknown): SyncExclusions {
  const record = asRecord(value);
  asString(record.document);
  asBoolean(record.usingDefaults);
  for (const raw of asArray(record.candidates)) {
    const candidate = asRecord(raw);
    asString(candidate.path);
    asBoolean(candidate.ignored);
  }
  return record as unknown as SyncExclusions;
}

function validateSyncBucketStatus(value: unknown): SyncBucketStatus {
  const record = asRecord(value);
  asString(record.checkedAt);
  asBoolean(record.localIsLive);
  asBoolean(record.historyTruncated);
  const validateObject = (value: unknown) => {
    const item = asRecord(value);
    asString(item.key);
    asNonnegativeInteger(item.size);
    if (item.lastModified !== undefined) asString(item.lastModified);
  };
  if (record.live !== undefined) validateObject(record.live);
  for (const item of asArray(record.history)) validateObject(item);
  return record as unknown as SyncBucketStatus;
}

function asRevision(value: unknown): string {
  const revision = asString(value);
  if (!/^[0-9a-f]{64}$/.test(revision)) throw new Error("invalid_response");
  return revision;
}

function validateSyncHistory(value: unknown): SyncHistory {
  const record = asRecord(value);
  asString(record.checkedAt);
  asRevision(record.headRevision);
  asBoolean(record.historyTruncated);
  asBoolean(record.downloadTruncated);
  asNonnegativeInteger(record.downloadedBytes);
  asNonnegativeInteger(record.skipped);
  for (const raw of asArray(record.revisions)) {
    const revision = asRecord(raw);
    asString(revision.key);
    asRevision(revision.revision);
    if (revision.parentRevision !== undefined)
      asRevision(revision.parentRevision);
    if (revision.message !== undefined) asString(revision.message);
    asString(revision.createdAt);
    asString(revision.origin);
    asNonnegativeInteger(revision.fileCount);
    asNonnegativeInteger(revision.size);
    if (revision.lastModified !== undefined) asString(revision.lastModified);
    if (!["head", "ancestor", "branch"].includes(asString(revision.relation))) {
      throw new Error("invalid_response");
    }
  }
  return record as unknown as SyncHistory;
}

function validateSyncHistoryDiff(value: unknown): SyncHistoryDiff {
  const record = asRecord(value);
  asRevision(record.fromRevision);
  asRevision(record.toRevision);
  for (const path of asArray(record.added)) asString(path);
  for (const path of asArray(record.modified)) asString(path);
  for (const path of asArray(record.removed)) asString(path);
  asNonnegativeInteger(record.downloadedBytes);
  return record as unknown as SyncHistoryDiff;
}

function validatePullResponse(value: unknown): PullResponse {
  const record = asRecord(value);
  asBoolean(record.applied);
  validateSnapshotSummary(record.summary);
  asNonnegativeInteger(record.downloadedBytes);
  asString(record.completedAt);
  asString(record.remoteETag);
  asRevision(record.remoteRevision);
  for (const conflict of asArray(record.conflicts)) {
    const entry = asRecord(conflict);
    asString(entry.path);
    asBoolean(entry.changedHere);
    asBoolean(entry.changedThere);
    for (const key of ["baseMode", "localMode", "remoteMode"] as const) {
      if (entry[key] === undefined) continue;
      const mode = asString(entry[key]);
      if (mode !== "0600" && mode !== "0700") {
        throw new Error("invalid_response");
      }
    }
  }
  for (const path of asArray(record.written)) asString(path);
  for (const path of asArray(record.removed)) asString(path);
  return record as unknown as PullResponse;
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
  const record = asRecord(value);
  return {
    name: asString(record.name),
    bytes: asNumber(record.bytes),
    type: asString(record.type),
  };
}

export const integrationsApi: IntegrationsApi = {
  async configCheck() {
    return validateConfigCheck(
      await postJSON<unknown>("/api/v1/diagnostics/config", {}),
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
      await postJSON<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/stream`,
        {},
      ),
    );
  },
  async reconnectTerminalSession(id) {
    return validateTerminalSessionList(
      await postJSON<unknown>(
        `/api/v1/terminal/sessions/${encodeURIComponent(id)}/reconnect`,
        {},
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
      await postJSON<unknown>("/api/v1/passwords/lock", {}),
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
    return validateSyncStatus(await postJSON<unknown>("/api/v1/sync/now", {}));
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
