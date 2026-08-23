import { en } from "./messages/en";
import { ja } from "./messages/ja";

export { en, ja };
export type { MessageKey } from "./messages/en";

export const messages = { en, ja } satisfies Record<string, Record<keyof typeof en, string>>;
