import { apiClient } from "./client";
import type { components } from "./schema";

export type ConfigCheckResponse = components["schemas"]["ConfigCheckResponse"];
export type EffectiveResponse = components["schemas"]["EffectiveResponse"];
export type ReachabilityResponse = components["schemas"]["ReachabilityResponse"];
export type AuthenticationResponse = components["schemas"]["AuthenticationResponse"];
export type TerminalSession = components["schemas"]["TerminalSession"];
export type TerminalSessionList = components["schemas"]["TerminalSessionList"];
export type OpenTerminalSessionRequest = components["schemas"]["OpenTerminalSessionRequest"];
export type OpenTerminalSessionResponse = components["schemas"]["OpenTerminalSessionResponse"];
export type TerminalStreamTicket = components["schemas"]["TerminalStreamTicket"];
export type KnownHostsResponse = components["schemas"]["KnownHostsResponse"];
export type KnownHostEntry = components["schemas"]["KnownHostEntry"];
export type KnownHostsChangeResponse = components["schemas"]["KnownHostsChangeResponse"];
export type KnownHostsScanResponse = components["schemas"]["KnownHostsScanResponse"];
export type KnownHostCandidate = components["schemas"]["KnownHostCandidate"];
export type IssueActionResponse = components["schemas"]["IssueActionResponse"];
export type ChangeMasterPasswordResult = components["schemas"]["ChangeMasterPasswordResult"];
export type LoginItem = components["schemas"]["LoginItem"];
export type UpdateStatus = components["schemas"]["UpdateStatus"];
export type PasswordVaultStatus = components["schemas"]["PasswordVaultStatus"];
export type PasswordEligibility = components["schemas"]["PasswordEligibility"];
export type Credential = components["schemas"]["Credential"];
export type CredentialList = components["schemas"]["CredentialList"];
// 二つの名前空間は string ではなく型として表現される。呼び出し側が
// 三つ目を作ったり、二つを入れ替えたりできないようにするためである。
export type CredentialKind = "password" | "key_passphrase";
export type SyncStatus = components["schemas"]["SyncStatus"];
export type SyncSettingsRequest = components["schemas"]["SyncSettingsRequest"];
export type SyncDirection = components["schemas"]["SyncDirection"];
export type SnapshotSummary = components["schemas"]["SnapshotSummary"];
export type SyncOperation = components["schemas"]["SyncOperation"];
export type PushResult = components["schemas"]["PushResult"];
export type PushResponse = components["schemas"]["PushResponse"];
export type PullResponse = components["schemas"]["PullResponse"];

// アクション語彙はサーバーのセッションパッケージに属し、操作を確認する
// すべてのサブシステムに対してそれを所有する。これらはその通信上の値である。
export const EVALUATE_ACTION_KIND = "diagnostics.evaluate";
export const REACHABILITY_ACTION_KIND = "diagnostics.reachability";
export const AUTHENTICATION_ACTION_KIND = "diagnostics.authentication";
export const KNOWN_HOSTS_DELETE_ACTION_KIND = "known_hosts.delete";
export const KNOWN_HOSTS_SCAN_ACTION_KIND = "known_hosts.scan";
export const KNOWN_HOSTS_ADD_ACTION_KIND = "known_hosts.add";

// KnownHostAddition はスキャン候補のうち、書き込まれる鍵を
// 特定する部分である。フィンガープリントはその一部として送られない。
// サーバーが鍵自体からフィンガープリントを導出し、ユーザーが入力したものと比較する。
export type KnownHostAddition = Pick<KnownHostCandidate, "host" | "port" | "keyType" | "key">;

export type IntegrationsApi = {
  configCheck(): Promise<ConfigCheckResponse>;
  effective(alias: string, confirm: boolean): Promise<EffectiveResponse>;
  reachability(alias: string): Promise<ReachabilityResponse>;
  authentication(alias: string, acknowledgeExecutable: boolean): Promise<AuthenticationResponse>;
  // 埋め込みターミナル。開くことに action token は要らない——vault ゲート
  // （マスターパスワード）だけが条件である。README がその代償を書いている。
  terminalSessions(): Promise<TerminalSessionList>;
  openTerminalSession(request: OpenTerminalSessionRequest): Promise<OpenTerminalSessionResponse>;
  terminalStreamTicket(id: string): Promise<TerminalStreamTicket>;
  // 改名は一覧の表示だけを変える。走っているプロセスにも ssh の相手にも
  // 触れず、metadata へも書かない。セッションと一緒に消える。
  renameTerminalSession(id: string, title: string): Promise<TerminalSessionList>;
  closeTerminalSession(id: string): Promise<TerminalSessionList>;
  knownHosts(query: string): Promise<KnownHostsResponse>;
  deleteKnownHosts(entries: { line: number; digest: string }[], path: string): Promise<KnownHostsChangeResponse>;
  scanKnownHosts(host: string, port: number): Promise<KnownHostsScanResponse>;
  addKnownHost(
    candidate: KnownHostAddition,
    expectedFingerprint: string,
    acknowledged: boolean,
  ): Promise<KnownHostsChangeResponse>;
  // vault。ここにあるどのメソッドもパスワードを返すことは決してない。
  // ステータスはパスワードを持つホストを運ぶだけであり、値が移動するのは
  // ブラウザからサーバーへ、またはサーバーから askpass ヘルパーへの経路だけである。
  passwordVault(): Promise<PasswordVaultStatus>;
  initialiseVault(passphrase: string): Promise<PasswordVaultStatus>;
  unlockVault(passphrase: string): Promise<PasswordVaultStatus>;
  lockVault(): Promise<PasswordVaultStatus>;
  changeMasterPassword(current: string, next: string): Promise<ChangeMasterPasswordResult>;
  updateStatus(): Promise<UpdateStatus>;
  loginItem(): Promise<LoginItem>;
  setLoginItem(enabled: boolean): Promise<LoginItem>;
  passwordEligibility(alias: string): Promise<PasswordEligibility>;
  // Credential は名前を持つ秘密である。ホストはアカウントパスワードを参照し、
  // 鍵はパスフレーズを参照する。この二つの名前空間は決して
  // 混ざらない。誤った方を選べば、鍵のパスフレーズをリモートホストへの
  // ログインパスワードとして送ってしまう。だから kind はうまく指定すべき
  // フィールドではなく、すべての呼び出しの一部である。
  credentials(): Promise<CredentialList>;
  storeCredential(kind: CredentialKind, name: string, secret: string): Promise<CredentialList>;
  deleteCredential(kind: CredentialKind, name: string): Promise<CredentialList>;
  assignCredential(kind: CredentialKind, subject: string, name: string): Promise<CredentialList>;
  unassignCredential(kind: CredentialKind, subject: string): Promise<CredentialList>;
  storePassword(alias: string, password: string): Promise<PasswordVaultStatus>;
  forgetPassword(alias: string): Promise<PasswordVaultStatus>;
  // リモートスナップショット。どのメソッドも資格情報やファイルの中身を返さない。
  // ステータスはエンドポイントとバケットを運び、pull はパスを運ぶ。
  syncStatus(): Promise<SyncStatus>;
  configureSync(settings: SyncSettingsRequest): Promise<SyncStatus>;
  pushSnapshot(passphrase: string): Promise<PushResponse>;
  pullSnapshot(passphrase: string, apply: boolean): Promise<PullResponse>;
};

// 生成された型は契約を記述するに過ぎない。これらの防護は
// UI が実際に受け取ったペイロードを検査する。型アサーションは実行時には何も証明しない。
function validateUpdate(value: unknown): UpdateStatus {
  const record = asRecord(value);
  if (typeof record.current !== "string" || typeof record.available !== "boolean") {
    throw new Error("invalid_response");
  }
  return {
    current: record.current,
    available: record.available,
    ...(typeof record.latest === "string" ? { latest: record.latest } : {}),
    ...(typeof record.pageUrl === "string" ? { pageUrl: record.pageUrl } : {}),
  };
}

function validateLoginItem(value: unknown): LoginItem {
  const record = asRecord(value);
  if (typeof record.enabled !== "boolean" || typeof record.supported !== "boolean") {
    throw new Error("invalid_response");
  }
  return { enabled: record.enabled, supported: record.supported };
}

function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("invalid_response");
  }
  return value as Record<string, unknown>;
}

function asArray(value: unknown): unknown[] {
  if (!Array.isArray(value)) throw new Error("invalid_response");
  return value;
}

function asString(value: unknown): string {
  if (typeof value !== "string") throw new Error("invalid_response");
  return value;
}

function asNumber(value: unknown): number {
  if (typeof value !== "number") throw new Error("invalid_response");
  return value;
}

function asNonnegativeInteger(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new Error("invalid_response");
  }
  return value;
}

function asBoolean(value: unknown): boolean {
  if (typeof value !== "boolean") throw new Error("invalid_response");
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
  asBoolean(record.evaluated);
  asBoolean(record.requiresConfirmation);
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
  asArray(record.values);
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
  const failure = asRecord(record.failure);
  asBoolean(failure.failed);
  asNumber(failure.exitCode);
  asString(failure.stderr);
  asBoolean(failure.truncated);
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
  asNumber(record.exitCode);
  asString(record.stderr);
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
  // alias を持つのは ssh のときだけである。localhost はローカルシェルであって
  // ssh 接続ではないので、alias を持たない。
  if (record.alias !== undefined) asString(record.alias);
  if (record.exited !== undefined) {
    const exited = asRecord(record.exited);
    asNumber(exited.code);
    asString(exited.signal);
    asString(exited.at);
  }
  return record as unknown as TerminalSession;
}

function validateTerminalSessionList(value: unknown): TerminalSessionList {
  const record = asRecord(value);
  for (const session of asArray(record.sessions)) validateTerminalSession(session);
  asNonnegativeInteger(record.maxSessions);
  return record as unknown as TerminalSessionList;
}

function validateOpenTerminalSession(value: unknown): OpenTerminalSessionResponse {
  const record = asRecord(value);
  validateTerminalSession(record.session);
  asString(record.streamTicket);
  return record as unknown as OpenTerminalSessionResponse;
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

const jsonHeaders = { "Content-Type": "application/json" } as const;

// issueAction はサーバーに確認の発行を求める。リクエストが
// 名指すのは操作と target だけであり、トークンが紐付く evidence は
// サーバー側で導出される。したがってこのクライアントは、ユーザーに一度も
// 見せていない状態にトークンを紐付けることができない。
async function issueAction(kind: string, target: string): Promise<string> {
  const response = await apiClient.mutate<IssueActionResponse>("/api/v1/actions", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ kind, target }),
  });
  return asString(asRecord(response).token);
}

async function postJSON<T>(path: string, body: unknown, actionToken?: string): Promise<T> {
  const headers: Record<string, string> = { ...jsonHeaders };
  if (actionToken) headers["X-SSHC-Action"] = actionToken;
  return apiClient.mutate<T>(path, { method: "POST", headers, body: JSON.stringify(body) });
}

function validateVaultStatus(value: unknown): PasswordVaultStatus {
  const record = asRecord(value);
  asBoolean(record.exists);
  asBoolean(record.unlocked);
  for (const alias of asArray(record.aliases)) asString(alias);
  for (const relativePath of asArray(record.dedicatedKeyPassphrases)) asString(relativePath);
  return record as unknown as PasswordVaultStatus;
}

// kind はパスセグメントであるため、呼び出し側が渡した任意の値を
// 埋め込むのではなく、閉じた集合からここで組み立てる。
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
  asString(record.endpoint);
  asString(record.bucket);
  asBoolean(record.synced);
  // direction はこのパネルがどのボタンを提示するかを決める。三つの
  // 値以外は、未知のモードとして表示するのではなく拒否する。
  const direction = asString(record.direction);
  if (direction !== "both" && direction !== "push" && direction !== "pull") {
    throw new Error(`unexpected sync direction: ${direction}`);
  }
  if (record.path !== undefined) asString(record.path);
  if (record.region !== undefined) asString(record.region);
  if (record.lastSyncedAt !== undefined) asString(record.lastSyncedAt);
  if (record.origin !== undefined) asString(record.origin);
  if (record.fileCount !== undefined) asNonnegativeInteger(record.fileCount);
  if (record.lastOperation !== undefined) validateSyncOperation(record.lastOperation);
  return record as unknown as SyncStatus;
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

function validatePullResponse(value: unknown): PullResponse {
  const record = asRecord(value);
  asBoolean(record.applied);
  validateSnapshotSummary(record.summary);
  asNonnegativeInteger(record.downloadedBytes);
  asString(record.completedAt);
  for (const conflict of asArray(record.conflicts)) {
    const entry = asRecord(conflict);
    asString(entry.path);
    asBoolean(entry.changedHere);
    asBoolean(entry.changedThere);
  }
  for (const path of asArray(record.written)) asString(path);
  for (const path of asArray(record.removed)) asString(path);
  return record as unknown as PullResponse;
}

export const integrationsApi: IntegrationsApi = {
  async configCheck() {
    return validateConfigCheck(await postJSON<unknown>("/api/v1/diagnostics/config", {}));
  },
  async effective(alias, confirm) {
    // 確認が消費されるのは、evaluate がコマンドを実行する場合
    // だけであり、安全な設定は確認なしで読める。
    const token = confirm ? await issueAction(EVALUATE_ACTION_KIND, alias) : undefined;
    return validateEffective(await postJSON<unknown>("/api/v1/diagnostics/effective", { alias }, token));
  },
  async reachability(alias) {
    const token = await issueAction(REACHABILITY_ACTION_KIND, alias);
    return validateReachability(await postJSON<unknown>("/api/v1/diagnostics/reachability", { alias }, token));
  },
  async authentication(alias, acknowledgeExecutable) {
    const token = await issueAction(AUTHENTICATION_ACTION_KIND, alias);
    return validateAuthentication(
      await postJSON<unknown>("/api/v1/diagnostics/authentication", { alias, acknowledgeExecutable }, token),
    );
  },
  async terminalSessions() {
    return validateTerminalSessionList(await apiClient.read("/api/v1/terminal/sessions"));
  },
  async openTerminalSession(request) {
    return validateOpenTerminalSession(
      await postJSON<unknown>("/api/v1/terminal/sessions", request),
    );
  },
  async terminalStreamTicket(id) {
    return validateStreamTicket(
      await postJSON<unknown>(`/api/v1/terminal/sessions/${encodeURIComponent(id)}/stream`, {}),
    );
  },
  async renameTerminalSession(id, title) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(`/api/v1/terminal/sessions/${encodeURIComponent(id)}`, {
        method: "PATCH",
        headers: { ...jsonHeaders },
        body: JSON.stringify({ title }),
      }),
    );
  },
  async closeTerminalSession(id) {
    return validateTerminalSessionList(
      await apiClient.mutate<unknown>(`/api/v1/terminal/sessions/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
    );
  },
  async passwordVault() {
    return validateVaultStatus(await apiClient.read("/api/v1/passwords"));
  },
  async initialiseVault(passphrase) {
    return validateVaultStatus(await postJSON<unknown>("/api/v1/passwords/initialise", { passphrase }));
  },
  async unlockVault(passphrase) {
    return validateVaultStatus(await postJSON<unknown>("/api/v1/passwords/unlock", { passphrase }));
  },
  async lockVault() {
    return validateVaultStatus(await postJSON<unknown>("/api/v1/passwords/lock", {}));
  },
  async updateStatus() {
    return validateUpdate(await apiClient.read("/api/v1/update"));
  },
  async loginItem() {
    return validateLoginItem(await apiClient.read("/api/v1/login-item"));
  },
  async setLoginItem(enabled) {
    return validateLoginItem(
      await apiClient.mutate("/api/v1/login-item", {
        method: "PUT",
        headers: jsonHeaders,
        body: JSON.stringify({ enabled, supported: true }),
      }),
    );
  },
  async changeMasterPassword(current, next) {
    const answer = await postJSON<unknown>("/api/v1/passwords/change", { current, next });
    const record = asRecord(answer);
    if (typeof record.snapshotResealed !== "boolean") throw new Error("invalid_response");
    return {
      vault: validateVaultStatus(record.vault),
      snapshotResealed: record.snapshotResealed,
      ...(typeof record.snapshotProblem === "string" ? { snapshotProblem: record.snapshotProblem } : {}),
    };
  },
  async passwordEligibility(alias) {
    return validatePasswordEligibility(
      await apiClient.read(`/api/v1/passwords/${encodeURIComponent(alias)}/eligibility`),
    );
  },
  async storePassword(alias, password) {
    return validateVaultStatus(
      await apiClient.mutate<unknown>(`/api/v1/passwords/${encodeURIComponent(alias)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      }),
    );
  },
  async forgetPassword(alias) {
    return validateVaultStatus(
      await apiClient.mutate<unknown>(`/api/v1/passwords/${encodeURIComponent(alias)}`, { method: "DELETE" }),
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
  async deleteCredential(kind, name) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, name), { method: "DELETE" }),
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
  async configureSync(settings) {
    return validateSyncStatus(
      await apiClient.mutate<unknown>("/api/v1/sync/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }),
    );
  },
  async pushSnapshot(passphrase) {
    return validatePushResponse(await postJSON<unknown>("/api/v1/sync/push", { passphrase }));
  },
  async pullSnapshot(passphrase, apply) {
    return validatePullResponse(await postJSON<unknown>("/api/v1/sync/pull", { passphrase, apply }));
  },
  async knownHosts(query) {
    return validateKnownHosts(await apiClient.read(`/api/v1/known-hosts?query=${encodeURIComponent(query)}`));
  },
  async deleteKnownHosts(entries, path) {
    const token = await issueAction(KNOWN_HOSTS_DELETE_ACTION_KIND, path);
    return validateChange(await postJSON<unknown>("/api/v1/known-hosts/delete", { entries }, token));
  },
  async scanKnownHosts(host, port) {
    const token = await issueAction(KNOWN_HOSTS_SCAN_ACTION_KIND, host);
    return validateScan(await postJSON<unknown>("/api/v1/known-hosts/scan", { host, port }, token));
  },
  // スキャンされた鍵が信頼されるのはここだけであり、ユーザーが与えた
  // 証明または承認があってのことである。確認はホストに
  // 紐付けられ、サーバーはそれに対してトークンを消費する。
  async addKnownHost(candidate, expectedFingerprint, acknowledged) {
    const token = await issueAction(KNOWN_HOSTS_ADD_ACTION_KIND, candidate.host);
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
