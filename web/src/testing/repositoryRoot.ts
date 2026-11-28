import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// リポジトリの最上位。webの外のファイルをテストから読むときの起点にする。
export const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
