import { Suspense, lazy, useEffect, useState, type ReactNode } from "react";
import type { TerminalAppearance, TerminalSettings } from "../api/settings";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { resolveAppearance } from "../terminal/appearance";
import { TerminalSplitView } from "../terminal/splitView/TerminalSplitView";
import type { SplitViewController } from "../terminal/splitView/useSplitView";
import type { RemotePathAction } from "../terminal/TerminalLinkPopover";
import type { TerminalSessionsState } from "../terminal/sessions";
import { RouteSkeleton } from "../ui/RouteSkeleton";
import type { PaneEdge } from "../terminal/splitView/layout";
import { SplitSessionPicker } from "../terminal/splitView/SplitSessionPicker";

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

// Terminals stay mounted behind other sections so output keeps flowing.
// Each session uses its host's appearance and clipboard policy.
export function TerminalScreen({
  visible,
  terminalSessions,
  activeSessionId,
  splitView,
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
  splitView: SplitViewController;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
  hostOSC52: Map<string, "allow" | "deny">;
  // hostVPN は、alias ごとに通るVPN経路の名前である。
  hostVPN: Map<string, string>;
  // api is the terminal API of the run target the sessions belong to.
  api: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath" | "terminalSessionDirectory">;
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
  const [splitRequest, setSplitRequest] = useState<{ targetSessionId: string; edge: PaneEdge } | null>(null);
  const session = terminalSessions.sessions.find((session) => session.id === activeSessionId) ?? null;
  useEffect(() => {
    document.title = session === null ? "sshc" : `${session.presentation?.displayTitle ?? session.title} · sshc`;
    return () => { document.title = "sshc"; };
  }, [session]);
  function renderTerminal(session: TerminalSession, headerActions: ReactNode) {
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
      <div className="flex min-h-0 flex-1 flex-col">
        <Suspense fallback={<RouteSkeleton kind="terminal" />}>
        <TerminalView
          key={session.id}
          session={session}
          active={visible && activeSessionId === session.id}
          focusRequest={splitView.state.focusRevision}
          headerActions={headerActions}
          {...(splitView.compact ? {} : { onSplit: (edge: PaneEdge) => setSplitRequest({ targetSessionId: session.id, edge }) })}
          canSplit={terminalSessions.sessions.some((candidate) => splitView.canSplit(candidate.id, session.id))}
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
  return <>
    <TerminalSplitView splitView={splitView} sessions={terminalSessions.sessions} renderTerminal={renderTerminal} />
    {splitRequest === null ? null : <SplitSessionPicker
      sessions={terminalSessions.sessions.filter((candidate) => splitView.canSplit(candidate.id, splitRequest.targetSessionId))}
      onClose={() => setSplitRequest(null)}
      onSelect={(sourceSessionId) => {
        splitView.split({ ...splitRequest, sourceSessionId });
        setSplitRequest(null);
      }} />}
  </>;
}
