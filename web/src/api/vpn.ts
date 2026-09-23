import { apiClient } from "./client";
import { postJSON, putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";
import { vpnRefusals } from "../vpn/vpnRefusals";

export type VPNOverview = components["schemas"]["VPNOverview"];
export type VPNProfileStatus = components["schemas"]["VPNProfileStatus"];
export type VPNProfile = components["schemas"]["VPNProfile"];
export type VPNSecrets = components["schemas"]["VPNSecrets"];
export type VPNLogs = components["schemas"]["VPNLogs"];

// ひとつのSSH接続だけを専用のVPNへ通す経路。トンネルはengineが持つコンテナの
// 中にあり、ブラウザーは設定と状態だけを扱う。秘密は保存のときだけ送り、
// 応答には現れない。
export type VPNApi = {
  vpnOverview(): Promise<VPNOverview>;
  // createVPNProfile は新しいプロファイルを作る。同じ名前があれば engine が断る。
  createVPNProfile(profile: VPNProfile, secrets: VPNSecrets): Promise<VPNOverview>;
  // saveVPNProfile は保存済みのプロファイルを更新する。空の秘密は保存済みの値を残す。
  saveVPNProfile(profile: VPNProfile, secrets?: VPNSecrets): Promise<VPNOverview>;
  removeVPNProfile(name: string): Promise<VPNOverview>;
  renameVPNProfile(from: string, to: string): Promise<VPNOverview>;
  vpnLogs(name: string): Promise<VPNLogs>;
  startVPNSession(name: string): Promise<VPNOverview>;
  stopVPNSession(name: string): Promise<VPNOverview>;
  setConnectionVPN(alias: string, profile: string): Promise<VPNOverview>;
};

function validateOverview(value: unknown): VPNOverview {
  return validateOpenAPISchema<VPNOverview>("VPNOverview", value);
}

function validateLogs(value: unknown): VPNLogs {
  return validateOpenAPISchema<VPNLogs>("VPNLogs", value);
}

function profilePath(name: string): string {
  return `/api/v1/vpn/profiles/${encodeURIComponent(name)}`;
}

// 経路の失敗は、この画面が自分で説明する。Dockerが無いことも、トンネルが
// 成立しないことも、利用者が次に何をするかを決める情報である。説明できるコードは
// 画面の言い方の表（vpnRefusals）と同じものなので、そこから作る。
const locallyExplainedVPNFailures = Object.keys(vpnRefusals);

export const vpnApi: VPNApi = {
  async vpnOverview() {
    return validateOverview(await apiClient.read("/api/v1/vpn"));
  },
  async createVPNProfile(profile, secrets) {
    return validateOverview(
      await postJSON<unknown>("/api/v1/vpn/profiles", { profile, secrets }, undefined, locallyExplainedVPNFailures),
    );
  },
  async saveVPNProfile(profile, secrets) {
    const body = secrets === undefined ? { profile } : { profile, secrets };
    return validateOverview(
      await putJSON<unknown>(profilePath(profile.name), body, locallyExplainedVPNFailures),
    );
  },
  async removeVPNProfile(name) {
    return validateOverview(
      await apiClient.mutate<unknown>(profilePath(name), { method: "DELETE" }, {
        locallyHandledCodes: locallyExplainedVPNFailures,
      }),
    );
  },
  async renameVPNProfile(from, to) {
    return validateOverview(
      await postJSON<unknown>(`${profilePath(from)}/rename`, { name: to }, undefined, locallyExplainedVPNFailures),
    );
  },
  async vpnLogs(name) {
    return validateLogs(
      await apiClient.read(`${profilePath(name)}/logs`, {
        locallyHandledCodes: locallyExplainedVPNFailures,
      }),
    );
  },
  async startVPNSession(name) {
    return validateOverview(
      // 本文は送らない。openapi はこの操作に本文を定めておらず、JSON を付けると
      // 送る前の検査（validateAPIRequest）が断る。
      await apiClient.mutate<unknown>(`${profilePath(name)}/session`, { method: "POST" }, {
        locallyHandledCodes: locallyExplainedVPNFailures,
      }),
    );
  },
  async stopVPNSession(name) {
    return validateOverview(
      await apiClient.mutate<unknown>(`${profilePath(name)}/session`, { method: "DELETE" }, {
        locallyHandledCodes: locallyExplainedVPNFailures,
      }),
    );
  },
  async setConnectionVPN(alias, profile) {
    return validateOverview(
      await putJSON<unknown>("/api/v1/vpn/bindings", { alias, profile }, locallyExplainedVPNFailures),
    );
  },
};
