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

// CreationPrerequisite は、新しい接続を作る途中で、先に用意しに行く画面である。
export type CreationPrerequisite = "Keys";
