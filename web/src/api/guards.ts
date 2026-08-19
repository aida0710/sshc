import { ApiError, apiClient, type Problem } from "./client";

// ここにあるのは、**サーバーが契約を破ったときに UI が壊れる前に止める**ための
// 検査である。
//
// TypeScript の型はコンパイル時にしか無い。openapi.yaml から生成した型を信じて
// `response.items.map(...)` と書けば、サーバーが `items` に null を返した日に、
// 画面は型の上では有り得ない場所で落ちる。ここを通せば、落ちるのは「応答が契約を
// 破っている」と名指しできる場所になる。
//
// **写しが 4 つあった。** api/config.ts、api/integrations.ts、keys/api.ts、
// remotekeys/api.ts である。**まだ食い違ってはいなかった**が、「null を record として
// 通さない」のような直しが 1 箇所にしか入らない状態は、いつでも起こりえた。

// asRecord は、これが JSON のオブジェクトであることを確かめる。
//
// **配列も null も通さない。** どちらも typeof では "object" である。
export function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("invalid_response");
  }
  return value as Record<string, unknown>;
}

export function asArray(value: unknown): unknown[] {
  if (!Array.isArray(value)) throw new Error("invalid_response");
  return value;
}

export function asString(value: unknown): string {
  if (typeof value !== "string") throw new Error("invalid_response");
  return value;
}

export function asNumber(value: unknown): number {
  if (typeof value !== "number") throw new Error("invalid_response");
  return value;
}

export function asBoolean(value: unknown): boolean {
  if (typeof value !== "boolean") throw new Error("invalid_response");
  return value;
}

// jsonHeaders は、本文を JSON として送ることを言う。
export const jsonHeaders = { "Content-Type": "application/json" } as const;

// toProblem は、投げられたものを画面が読める理由に均す。
//
// **画面ごとに書くものではない。** ApiError を Problem に正規化するのは、あの型を
// 持っている側の仕事である——4 つの画面がそれぞれ同じ 4 行を持っていた。
export function toProblem(error: unknown): Problem {
  if (error instanceof ApiError && error.problem !== null) return error.problem;
  if (error instanceof ApiError) return { code: error.code, message: "request rejected" };
  return { code: "request_failed", message: "request rejected" };
}

// issueAction は、サーバーに確認の発行を求める。
//
// **要求が名指すのは操作と対象だけである。** トークンが縛られる evidence は
// サーバー側で導出されるので、このクライアントは、**利用者に一度も見せていない状態
// にトークンを紐付けることができない。**
export async function issueAction(kind: string, target: string): Promise<string> {
  const response = await apiClient.mutate<unknown>("/api/v1/actions", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ kind, target }),
  });
  return asString(asRecord(response).token);
}
