import { apiClient } from "../api/client";
import { asRecord, asArray, asString, asNumber, asBoolean, jsonHeaders } from "../api/guards";
import type { components } from "../api/schema";

export type RemoteKeyPlan = components["schemas"]["RemoteKeyPlan"];
export type RemoteKeyRegisterResponse = components["schemas"]["RemoteKeyRegisterResponse"];
export type ExecutableDirective = components["schemas"]["ExecutableDirective"];

export type RemoteKeyInput = {
  alias: string;
  keyPath: string;
  publicKey: string;
};

export type RemoteKeyRegisterInput = RemoteKeyInput & {
  acknowledgeExecutable: boolean;
  actionToken: string;
};

export type RemoteKeysApi = {
  plan(input: RemoteKeyInput): Promise<RemoteKeyPlan>;
  register(input: RemoteKeyRegisterInput): Promise<RemoteKeyRegisterResponse>;
};






function validatePlan(value: unknown): RemoteKeyPlan {
  const record = asRecord(value);
  asString(record.alias);
  asString(record.user);
  asString(record.hostname);
  asString(record.port);
  asString(record.valuesFrom);
  asString(record.fingerprint);
  asString(record.keyPath);
  asString(record.keyLine);
  asString(record.remotePath);
  asString(record.routine);
  asBoolean(record.supported);
  for (const step of asArray(record.manual)) asString(step);
  for (const directive of asArray(record.executableDirectives)) {
    const entry = asRecord(directive);
    asString(entry.keyword);
    asString(entry.command);
    asString(entry.path);
    asNumber(entry.line);
    asBoolean(entry.overridable);
  }
  asString(record.actionToken);
  asString(record.actionExpiresAt);
  return record as unknown as RemoteKeyPlan;
}

function validateRegistration(value: unknown): RemoteKeyRegisterResponse {
  const record = asRecord(value);
  asString(record.outcome);
  asNumber(record.exitCode);
  asString(record.stderr);
  asBoolean(record.truncated);
  return record as unknown as RemoteKeyRegisterResponse;
}


export const remoteKeysApi: RemoteKeysApi = {
  // plan は設定を読むだけで何にも接続しないので、確認を消費しない。
  // これが存在するのは、実行を求められるようになる前に、ユーザーが
  // 変更内容を見られるようにするためだ。
  async plan(input) {
    return validatePlan(
      await apiClient.mutate<unknown>("/api/v1/remote-keys/plan", {
        method: "POST",
        headers: jsonHeaders,
        body: JSON.stringify({ alias: input.alias, keyPath: input.keyPath, publicKey: input.publicKey }),
      }),
    );
  },
  async register(input) {
    return validateRegistration(
      await apiClient.mutate<unknown>("/api/v1/remote-keys/register", {
        method: "POST",
        headers: { ...jsonHeaders, "X-SSHC-Action": input.actionToken },
        body: JSON.stringify({
          alias: input.alias,
          keyPath: input.keyPath,
          publicKey: input.publicKey,
          acknowledgeExecutable: input.acknowledgeExecutable,
        }),
      }),
    );
  },
};
