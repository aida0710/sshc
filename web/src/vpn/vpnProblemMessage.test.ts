import { describe, expect, it } from "vitest";
import { ApiError, type Problem } from "../api/client";
import type { Translate } from "../i18n/context";
import { en, ja, type MessageKey } from "../i18n/messages";
import { describeVPNProblem } from "./vpnProblemMessage";
import { vpnProblemCodes } from "./vpnRefusals";

// translator は、画面の t と同じく、その言語の文の {name} を値で埋める。
function translator(catalogue: Record<MessageKey, string>): Translate {
  return (key, values = {}) =>
    catalogue[key].replace(/\{(\w+)\}/g, (whole, name: string) => (name in values ? String(values[name]) : whole));
}

function refusal(code: string, extra: Partial<Problem> = {}): ApiError {
  return new ApiError(code, 409, { code, message: "request rejected", ...extra });
}

describe("describeVPNProblem", () => {
  const t = translator(ja);

  it("says why the VPN could not reach the destination, the way the CLI says it", () => {
    expect(describeVPNProblem(t, refusal("vpn_target_failed", { reason: "target_unresolved" }))).toBe(
      "VPN経由で接続先に接続できませんでした。VPN内のDNSサーバーで接続先の名前解決に失敗しました。DNSサーバーとHostNameを確認してください。",
    );
  });

  it("says why the destination cannot be used through the VPN", () => {
    expect(describeVPNProblem(t, refusal("vpn_destination_invalid", { reason: "name_needs_dns" }))).toBe(
      "接続先をホスト名で指定する場合は、VPNプロファイルにVPN内のDNSサーバーを指定してください。",
    );
  });

  it("says a route failure and its reason in one sentence", () => {
    expect(describeVPNProblem(t, refusal("vpn_session_failed", { reason: "ppp_authentication" }))).toBe(
      "VPNの接続に失敗しました。PPPの認証に失敗しました。ユーザー名とパスワードを確認してください。",
    );
  });

  it("points to the logs when the reason is one this screen does not know", () => {
    expect(describeVPNProblem(t, refusal("vpn_target_failed", { reason: "new_reason" }))).toBe(
      "VPN経由で接続先に接続できませんでした。原因を特定できませんでした。ログを確認してください。",
    );
  });

  it("names the field an invalid value was in", () => {
    expect(describeVPNProblem(t, refusal("vpn_profile_invalid", { field: "name", reason: "too_long", limit: 48 }))).toBe(
      "名前: 長すぎます（48文字まで）。",
    );
  });

  it("falls back to the refusal's own sentence when the field is not named", () => {
    expect(describeVPNProblem(t, refusal("vpn_profile_invalid"))).toBe(
      "VPNプロファイルに使用できない値があります。VPNサーバーとDNSサーバーの指定を確認してください。",
    );
  });

  it("says Docker is not running, apart from Docker being missing", () => {
    expect(describeVPNProblem(t, refusal("vpn_docker_not_running"))).toMatch(/^Dockerが起動していません。/);
    expect(describeVPNProblem(t, refusal("vpn_docker_missing"))).toMatch(/^Dockerが見つかりません。/);
  });

  it("has a sentence for every code the engine sends as a VPN refusal, in both languages", () => {
    for (const catalogue of [ja, en]) {
      for (const code of vpnProblemCodes) {
        expect(describeVPNProblem(translator(catalogue), refusal(code)), code).not.toBeNull();
      }
    }
  });

  it("leaves problems that are not VPN refusals to the caller", () => {
    expect(describeVPNProblem(t, refusal("sftp_failed", { field: "name", reason: "format" }))).toBeNull();
    expect(describeVPNProblem(t, new Error("offline"))).toBeNull();
  });
});
