import { Suspense, lazy, useEffect } from "react";
import type { TerminalAppearance, TerminalSettings } from "../api/settings";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { resolveAppearance } from "../terminal/appearance";
import { TerminalEmptyState } from "../terminal/TerminalEmptyState";
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

// The terminal pane: the active session, with the appearance and clipboard
// policy its host asks for. It stays mounted while other sections show, so
// that output keeps flowing and nothing is re-rendered on return.
export function TerminalScreen({
  visible,
  terminalSessions,
  activeSessionId,
  settings,
  hostAppearance,
  hostOSC52,
  hostVPN,
  api,
  runTarget,
  onOpenRemotePath,
  onOSC52Change,
}: {
  visible: boolean;
  terminalSessions: TerminalSessionsState;
  activeSessionId: string | null;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
  hostOSC52: Map<string, "allow" | "deny">;
  // hostVPN は、alias ごとに通るVPN経路の名前である。
  hostVPN: Map<string, string>;
  // api is the terminal API of the run target the sessions belong to.
  api: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath">;
  // runTarget is the SSH alias of that engine; null for this machine.
  runTarget: string | null;
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
  const session = terminalSessions.sessions.find((session) => session.id === activeSessionId) ?? null;
  useEffect(() => {
    document.title = session === null ? "sshc" : `${session.presentation?.displayTitle ?? session.title} · sshc`;
    return () => { document.title = "sshc"; };
  }, [session]);
  if (session === null) return <TerminalEmptyState />;
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
  const vpnProfile = session.alias === undefined ? "" : hostVPN.get(session.alias) ?? "";
  return (
    <div className="flex h-full min-h-0 flex-col">
      <Suspense fallback={<RouteSkeleton kind="terminal" />}>
      <TerminalView
        key={session.id}
        session={session}
        api={api}
        {...(runTarget === null ? {} : { runTarget })}
        searchShortcutActive={visible && activeSessionId === session.id}
        {...(settings.fontSize === undefined
          ? {}
          : { fontSize: settings.fontSize })}
        {...(settings.browserScrollbackLines === undefined
          ? {}
          : { scrollbackLines: settings.browserScrollbackLines })}
        osc52Enabled={osc52Enabled}
        vpnProfile={vpnProfile}
        jisYenBackslash={settings.jisYenBackslash ?? false}
        onOsc52Change={(enabled) => onOSC52Change(session, enabled)}
        onForwardsChanged={terminalSessions.refresh}
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
        onExit={() => terminalSessions.markExited(session.id)}
        onReconnect={() => terminalSessions.reconnect(session.id)}
        onStopReconnect={() => terminalSessions.stopReconnect(session.id)}
        onOpenRemotePath={onOpenRemotePath}
      />
      </Suspense>
    </div>
  );
}
