import { configApi } from "../api/config";
import { connectionAliasFromHostName, uniqueConnectionAlias } from "../connections/connectionAliasFromHost";
import type { DirectDestination } from "../connections/directDestination";
import { useTranslate } from "../i18n/context";
import { useToast } from "../ui/useToast";

// The name of a saved destination whose host name suggests none (the IPv6 address ::).
const fallbackAliasBase = "host";

// DirectDestinationActions is what a search can do with a user@host[:port] typed into it. workspaceId is where the
// session goes; without one the engine puts it in the default workspace.
export type DirectDestinationActions = {
  connect: (destination: DirectDestination, workspaceId?: string) => void;
  // saveAndConnect is left out where a saved connection could not be opened (a run target reads its own config files).
  saveAndConnect?: ((destination: DirectDestination, workspaceId?: string) => void) | undefined;
  // aliasFor is the connection name saving would give.
  aliasFor: (destination: DirectDestination) => string;
};

// useDirectDestinationActions connects to a destination as it is, or writes its Host block first and then connects to
// that: a saved destination is an ordinary connection in the config files. A password is asked for when connecting.
export function useDirectDestinationActions({ aliases, canSave, open, onSaved }: {
  aliases: readonly string[];
  canSave: boolean;
  open: (alias: string, cwd?: string, workspaceId?: string) => Promise<void>;
  onSaved: () => void;
}): DirectDestinationActions {
  const t = useTranslate();
  const showToast = useToast();

  function aliasFor(destination: DirectDestination): string {
    return uniqueConnectionAlias(connectionAliasFromHostName(destination.host) || fallbackAliasBase, aliases);
  }

  async function saveAndConnect(destination: DirectDestination, workspaceId?: string) {
    try {
      const created = await configApi.createConnection({
        alias: aliasFor(destination),
        hostName: destination.host,
        user: destination.user,
        ...(destination.port === undefined ? {} : { port: destination.port }),
        authentication: { kind: "ask_on_connect" },
      });
      onSaved();
      await open(created.identity.alias, undefined, workspaceId);
    } catch {
      showToast(t("directDestination.saveFailed"));
    }
  }

  return {
    aliasFor,
    connect: (destination, workspaceId) => void open(destination.text, undefined, workspaceId),
    saveAndConnect: canSave ? (destination, workspaceId) => void saveAndConnect(destination, workspaceId) : undefined,
  };
}
