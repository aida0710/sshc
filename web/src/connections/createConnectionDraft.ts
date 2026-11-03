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
  authentication: "dedicated_password",
  savedCredential: "",
  newCredential: "",
  keyId: "",
};

// CreationPrerequisite は、新しい接続を作る途中で、先に用意しに行く画面である。
export type CreationPrerequisite = "Keys";
