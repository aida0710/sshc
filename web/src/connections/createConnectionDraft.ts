import type { CreateConnectionAuthentication } from "../api/config";

export type CreateConnectionAuthenticationKind = CreateConnectionAuthentication["kind"];

// CreateConnectionDraft は、作りかけの新しい接続のうち、ほかの画面へ移っても持ち越してよい値である。
// パスワードは入れない。
export type CreateConnectionDraft = {
  alias: string;
  group: string;
  hostName: string;
  user: string;
  port: string;
  // proxyJump is the connection to go through, noProxyJump against the group's default, or "" to write nothing.
  proxyJump: string;
  authentication: CreateConnectionAuthenticationKind;
  savedCredential: string;
  newCredential: string;
  keyId: string;
};

// blankConnectionDraft は、何も決まっていない新しい接続である。ほかの画面からこれを渡して Connections を開くと、
// 新しい接続の入力が開く。
export const blankConnectionDraft: CreateConnectionDraft = {
  alias: "",
  group: "",
  hostName: "",
  user: "",
  port: "",
  proxyJump: "",
  authentication: "dedicated_password",
  savedCredential: "",
  newCredential: "",
  keyId: "",
};

// noProxyJump is `ProxyJump none`: connect directly although the group goes through a jump host.
export const noProxyJump = "none";

// authenticationNeedsVault tells whether this way of authenticating keeps a password in the Vault.
export function authenticationNeedsVault(kind: CreateConnectionAuthenticationKind): boolean {
  return kind !== "identity_file" && kind !== "ask_on_connect";
}

// CreationPrerequisite は、新しい接続を作る途中で、先に用意しに行く画面である。
export type CreationPrerequisite = "Keys";
