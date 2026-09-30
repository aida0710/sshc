import type { Translate } from "../i18n/context";
import { isValidAlias, isValidHostName } from "../rules/rules";

// The four fields of a new connection that are typed in, as the form holds them.
export type CreateConnectionFields = {
  alias: string;
  hostName: string;
  user: string;
  port: string;
};

export type CreateConnectionFieldErrors = Record<keyof CreateConnectionFields, string>;

// createConnectionFieldErrors は、入力した名前・ホスト名・ユーザー名・ポートの誤りを
// 項目ごとの文にする。誤りの無い項目は "" である。ユーザー名とポートは空でよい。
export function createConnectionFieldErrors(t: Translate, fields: CreateConnectionFields): CreateConnectionFieldErrors {
  const { alias, hostName, user, port } = fields;
  const parsedPort = Number(port);
  return {
    alias: alias === "" ? t("conn.createAliasRequired") : isValidAlias(alias) ? "" : t("conn.createAliasInvalid"),
    hostName: hostName === "" ? t("conn.createHostRequired") : isValidHostName(hostName) ? "" : t("conn.createHostInvalid"),
    user: user !== "" && /[\s\p{Cc}]/u.test(user) ? t("conn.createUserInvalid") : "",
    port: port !== "" && (!/^\d+$/.test(port) || parsedPort < 1 || parsedPort > 65535) ? t("conn.createPortInvalid") : "",
  };
}
