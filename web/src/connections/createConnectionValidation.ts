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

// aliasProblem says what is wrong with the connection name. An empty name takes the one the host name suggests; only a
// host name that suggests none needs a name typed. An invalid host name is said at its own field.
function aliasProblem(t: Translate, { alias, hostName }: CreateConnectionFields, suggestedAlias: string): string {
  if (alias !== "") return isValidAlias(alias) ? "" : t("conn.createAliasInvalid");
  return isValidHostName(hostName) && suggestedAlias === "" ? t("conn.createAliasRequired") : "";
}

// createConnectionFieldErrors は、入力した名前・ホスト名・ユーザー名・ポートの誤りを
// 項目ごとの文にする。誤りの無い項目は "" である。ユーザー名とポートは空でよい。
export function createConnectionFieldErrors(
  t: Translate,
  fields: CreateConnectionFields,
  suggestedAlias: string,
): CreateConnectionFieldErrors {
  const { hostName, user, port } = fields;
  const parsedPort = Number(port);
  return {
    alias: aliasProblem(t, fields, suggestedAlias),
    hostName: hostName === "" ? t("conn.createHostRequired") : isValidHostName(hostName) ? "" : t("conn.createHostInvalid"),
    user: user !== "" && /[\s\p{Cc}]/u.test(user) ? t("conn.createUserInvalid") : "",
    port: port !== "" && (!/^\d+$/.test(port) || parsedPort < 1 || parsedPort > 65535) ? t("conn.createPortInvalid") : "",
  };
}
