// 英語をキーの正本とし、各言語は個別ファイルで管理する。
// このファイルは既存のimport先を維持する公開窓口に限定する。
import { en } from "./messages/en";
import { ja } from "./messages/ja";

export { en, ja };
export type { MessageKey } from "./messages/en";

export const messages = { en, ja } satisfies Record<string, Record<keyof typeof en, string>>;
