import { useCallback } from "react";
import { engineTerminalBase } from "../api/remoteEngines";
import { fetchTerminalDestination, terminalDestinationBase, type TerminalDestination } from "../api/terminalDestination";
import { useLoad } from "../ui/useLoad";

function sameDestination(here: TerminalDestination, there: TerminalDestination): boolean {
  return here.hostName === there.hostName && here.port === there.port && here.user === there.user;
}

// useSameDestination answers whether an SSH session a run target opened as alias reached the host this
// machine reaches with the same alias: both engines resolve alias, and HostName, Port and User must match.
// Only then may this machine read that session's files (path preview, SFTP); otherwise they would come
// from another machine. It stays false while either answer is pending or either engine cannot resolve it.
export function useSameDestination(alias: string | undefined, runTarget: string | undefined): boolean {
  const compare = useCallback(async () => {
    if (alias === undefined || runTarget === undefined) return false;
    const [here, there] = await Promise.all([
      fetchTerminalDestination(terminalDestinationBase, alias),
      fetchTerminalDestination(engineTerminalBase(runTarget), alias),
    ]);
    return sameDestination(here, there);
  }, [alias, runTarget]);
  const compared = useLoad(compare);
  return compared.state === "loaded" && compared.value;
}
