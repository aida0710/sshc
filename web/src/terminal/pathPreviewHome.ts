import { sftpApi } from "../sftp/api";
import { localHostAlias } from "../sftp/localHost";

// Use the same machine and home semantics as the file browser. A run target
// is an SSH alias here, just as it is when reading the preview itself.
export async function pathPreviewHome(alias: string): Promise<string> {
  if (alias === localHostAlias) return (await sftpApi.listLocal("~")).home;
  return (await sftpApi.list(alias, "~")).path;
}
