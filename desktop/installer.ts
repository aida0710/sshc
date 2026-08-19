import { join } from "node:path";

// windowsCLISubdirectory は、束の中で Go の CLI が置かれる場所である。
//
// **NSIS の installer.nsh と対である。** あちらは同じ相対パスを利用者の PATH
// へ足す。片方だけを変えると、インストールは成功したのに `sshc` と打っても
// 何も無い、という壊れ方をする。
export const windowsCLISubdirectory = ["cli", "sshc.exe"];

/**
 * engineBinary は、束に同梱された sshc の場所を返す。
 *
 * **OS ごとに違う。** macOS と Linux は resources の直下に一つだけ置くが、
 * Windows は `resources\cli\sshc.exe` に置く——そのディレクトリごと利用者の
 * PATH へ足すので、そこに Electron の資材が混ざっていてはならない。
 */
export function engineBinary({
  platform,
  resourcesPath,
}: {
  platform: NodeJS.Platform | string;
  resourcesPath?: string | undefined;
}): string {
  const root = resourcesPath ?? "";
  return platform === "win32"
    ? join(root, ...windowsCLISubdirectory)
    : join(root, "sshc");
}

/**
 * managesItsOwnCLI は、外殻が端末側の入口を自分で用意するかを言う。
 *
 * **Windows では否である。** あちらの安定した場所を作るのはインストーラで
 * あり、PATH を通すのも同じインストーラである。外殻が重ねて symlink を張ろう
 * とすれば、開発者モードか管理者権限が要るうえ、`~/.local/bin` は PATH に
 * 載っていない——できないことを試みて、できなかったと報告するだけになる。
 */
export function managesItsOwnCLI(platform: NodeJS.Platform | string): boolean {
  return platform !== "win32";
}
