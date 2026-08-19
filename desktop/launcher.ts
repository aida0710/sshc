import * as promises from "node:fs/promises";
import { join, dirname, isAbsolute } from "node:path";
import { homedir } from "node:os";
import { atomicReplace, type FileSystem } from "./install-cli.js";

// descriptorPath は、外殻が自分の居場所を書き残す先である。
//
// **Go 側の cmd/sshc/launch_linux.go が読む唯一の場所である。** 名前も形も
// あちらと対であり、片方だけを変えると、端末で `sshc` と打った人は起こし方を
// 失う。
export function descriptorPath(): string {
  return join(homedir(), ".ssh", "sshc", "desktop.json");
}

/**
 * recordLinuxLauncher は、この AppImage の元の場所を書き残す。
 *
 * **process.execPath ではなく APPIMAGE を書く。** 走っているのは一時マウントの
 * 中身で、その道はアプリを閉じれば消える。端末から起こせる名前は、利用者が
 * ダウンロードした AppImage そのものだけである。
 *
 * **相対パスは書かない。** 書けば、読んだ側にとって意味が作業ディレクトリで
 * 変わる——起こすものが、どこで打ったかで変わってはならない。
 */
export async function recordLinuxLauncher({
  appImage = process.env["APPIMAGE"],
  descriptor = descriptorPath(),
  fs = promises,
  platform = process.platform,
  packaged = true,
}: {
  appImage?: string | undefined;
  descriptor?: string;
  fs?: FileSystem;
  platform?: NodeJS.Platform | string;
  packaged?: boolean;
}): Promise<{ recorded: boolean; reason: string }> {
  if (platform !== "linux") {
    return { recorded: false, reason: "only Linux is launched from a path" };
  }
  if (!packaged) {
    // 開発中に走っているのは checkout の Electron であって、利用者が後から
    // 起こせるものではない。書けば、次に端末で打った人が消えた道を起こす。
    return { recorded: false, reason: "a development run has no stable path" };
  }
  if (
    typeof appImage !== "string" ||
    appImage === "" ||
    !isAbsolute(appImage)
  ) {
    return { recorded: false, reason: "APPIMAGE is not an absolute path" };
  }

  await fs.mkdir(dirname(descriptor), { recursive: true, mode: 0o700 });
  // 資格情報は入らない文書だが、利用者だけのものである。
  await atomicReplace(
    fs,
    descriptor,
    `${JSON.stringify({ executable: appImage })}\n`,
    0o600,
  );
  return { recorded: true, reason: "" };
}
