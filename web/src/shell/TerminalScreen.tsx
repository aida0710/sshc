import { Suspense, lazy } from "react";
import type { TerminalAppearance, TerminalSettings } from "../api/settings";
import type { TerminalSession } from "../api/terminalSessions";
import { TerminalWorkspace, type WorkspaceRenameRequest, type WorkspaceRestoreRequest } from "../features/workspaces/TerminalWorkspace";
import type { LiveWorkspaceSummary } from "../features/workspaces/live";
import { resolveAppearance } from "../terminal/appearance";
import type { RemotePathAction } from "../terminal/TerminalLinkPopover";
import type { TerminalSessionsState } from "../terminal/sessions";
import { RouteSkeleton } from "../ui/RouteSkeleton";

const TerminalView = lazy(() =>
  import("../terminal/TerminalView").then(({ TerminalView }) => ({ default: TerminalView })),
);

// A host's own OSC 52 policy wins over the terminal-wide default.
export function resolveOSC52(
  policy: "allow" | "deny" | undefined,
  fallback: boolean,
): boolean {
  return policy === undefined ? fallback : policy === "allow";
}

// The terminal pane: every live console, with the appearance and clipboard
// policy each host asks for. It stays mounted while other sections show, so
// that output keeps flowing and nothing is re-rendered on return.
export function TerminalScreen({
  visible,
  consoles,
  activeConsole,
  settings,
  hostAppearance,
  hostOSC52,
  onActive,
  onLiveWorkspaceChange,
  onOpenAlias,
  onOpenShell,
  restoreRequest,
  onRestoreConsumed,
  renameRequest,
  onRenameConsumed,
  onOpenRemotePath,
  onOSC52Change,
}: {
  visible: boolean;
  consoles: TerminalSessionsState;
  activeConsole: string | null;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
  hostOSC52: Map<string, "allow" | "deny">;
  onActive: (id: string) => void;
  onLiveWorkspaceChange: (workspace: LiveWorkspaceSummary | null) => void;
  onOpenAlias: (
    alias: string,
  ) => Promise<TerminalSession | null>;
  onOpenShell: () => Promise<
    TerminalSession | null
  >;
  restoreRequest: WorkspaceRestoreRequest | null;
  onRestoreConsumed: (sequence: number) => void;
  renameRequest: WorkspaceRenameRequest | null;
  onRenameConsumed: (sequence: number) => void;
  onOpenRemotePath: (
    alias: string,
    path: string,
    action: RemotePathAction,
  ) => void;
  onOSC52Change: (
    session: TerminalSession,
    enabled: boolean,
  ) => Promise<void>;
}) {
  return (
    <TerminalWorkspace
      sessions={consoles.sessions}
      sessionsLoaded={consoles.loaded}
      activeSessionId={activeConsole}
      onActive={onActive}
      onOpenAlias={onOpenAlias}
      onOpenShell={onOpenShell}
      onClose={consoles.close}
      restoreRequest={restoreRequest}
      onRestoreConsumed={onRestoreConsumed}
      renameRequest={renameRequest}
      onRenameConsumed={onRenameConsumed}
      onLiveWorkspaceChange={onLiveWorkspaceChange}
      renderTerminal={(session) => {
        const appearance = resolveAppearance(
          session.alias === undefined
            ? undefined
            : hostAppearance.get(session.alias),
          settings.appearance,
        );
        const hostPolicy =
          session.alias === undefined
            ? undefined
            : hostOSC52.get(session.alias);
        const osc52Enabled = resolveOSC52(hostPolicy, settings.osc52 ?? false);
        return (
          <Suspense fallback={<RouteSkeleton kind="terminal" />}>
            <TerminalView
              key={session.id}
              session={session}
              searchShortcutActive={visible && activeConsole === session.id}
              {...(settings.fontSize === undefined
                ? {}
                : { fontSize: settings.fontSize })}
              {...(settings.browserScrollbackLines === undefined
                ? {}
                : { scrollbackLines: settings.browserScrollbackLines })}
              osc52Enabled={osc52Enabled}
              jisYenBackslash={settings.jisYenBackslash ?? false}
              onOsc52Change={(enabled) => onOSC52Change(session, enabled)}
              onForwardsChanged={consoles.refresh}
              {...(appearance.palette === ""
                ? {}
                : { palette: appearance.palette })}
              {...(appearance.font === "" ? {} : { font: appearance.font })}
              {...(appearance.background === ""
                ? {}
                : { background: appearance.background })}
              {...(appearance.tint === undefined
                ? {}
                : { tint: appearance.tint })}
              copyOnSelect={settings.copyOnSelect ?? true}
              rightClickPaste={settings.rightClickPaste ?? true}
              webgl={settings.webgl ?? true}
              onExit={() => consoles.markExited(session.id)}
              onReconnect={() => consoles.reconnect(session.id)}
              onStopReconnect={() => consoles.stopReconnect(session.id)}
              onOpenRemotePath={onOpenRemotePath}
            />
          </Suspense>
        );
      }}
    />
  );
}
