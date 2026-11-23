import { useEngineApis } from "../shell/engineApis";
import { sftpTransferManager, transferManagerFor, type SFTPTransferManager } from "./transferManager";

// useSftpTransferManager は、この部品の木が相手にする sshc エンジンの、転送の管理を返す。
export function useSftpTransferManager(): SFTPTransferManager {
  const { runTarget } = useEngineApis();
  return runTarget === null ? sftpTransferManager : transferManagerFor(runTarget);
}
