import { useEngineOS } from "../about/useEngineOS";
import type { TerminalSession } from "../api/terminalSessions";

// ブラウザの OS とターミナルの出力からは、プレビューが読むファイルの場所が分からない。シェルのファイルは、
// そのシェルを動かす sshc エンジンが自分のマシンのものとして読み、Windows のエンジンだけが Windows のパスを受け付ける。
// SSH のセッションは接続先のファイルを SFTP で読むので、Windows のパスは使わない。
export function useLocalWindowsPaths(kind: TerminalSession["kind"], runTarget: string | undefined): boolean {
  const shell = kind === "shell";
  const os = useEngineOS({ enabled: shell, runTarget: runTarget ?? null });
  return shell && os === "windows";
}
