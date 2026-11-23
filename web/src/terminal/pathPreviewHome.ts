import type { SftpApi } from "../sftp/api";
import { localHostAlias } from "../sftp/localHost";

// Use the same machine and home semantics as the file browser, with the API of the engine that runs the session.
export async function pathPreviewHome(sftpApi: Pick<SftpApi, "list" | "listLocal">, alias: string): Promise<string> {
  if (alias === localHostAlias) return (await sftpApi.listLocal("~")).home;
  return (await sftpApi.list(alias, "~")).path;
}
