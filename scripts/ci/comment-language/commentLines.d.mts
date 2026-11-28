// commentLines.mjs の型。web/src のテスト（使わないと決めた語の検査）が TypeScript から読むので置く。

export type Language = "go" | "java" | "kotlin" | "javascript" | "css";

// CommentLine は、コメントのある1行の、コメントの断片と、コメントを除いた残りである。
export type CommentLine = { pieces: string[]; code: string; commentOnly: boolean };

export function languageOfPath(path: string): Language | undefined;

export function commentLinesOf(source: string, language: Language): Map<number, CommentLine>;
