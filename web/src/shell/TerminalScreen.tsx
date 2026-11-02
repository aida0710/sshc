import { Suspense, lazy, useEffect, useState, type ReactNode } from "react";
import type { TerminalAppearance, TerminalSettings } from "../api/settings";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { resolveAppearance } from "../terminal/appearance";
import { TerminalSplitView } from "../terminal/splitView/TerminalSplitView";
import { defaultRetainedTerminals } from "../terminal/splitView/retainedSessions";
import type { SplitViewController } from "../terminal/splitView/useSplitView";
import type { RemotePathAction } from "../sftp/SFTPPanel";
import type { TerminalSessionsState } from "../terminal/sessions";
import { RouteSkeleton } from "../ui/RouteSkeleton";
import type { PaneEdge } from "../terminal/splitView/layout";
import { SplitSessionPicker } from "../terminal/splitView/SplitSessionPicker";
import { RunTargetConnectingPanel } from "../terminal/RunTargetConnectingPanel";
import { RunTargetRefusalPanel } from "../terminal/RunTargetRefusalPanel";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { useKeepScreenAwake } from "../terminal/keepScreenAwake";
import { useScreenWakeLock } from "../terminal/useScreenWakeLock";

const TerminalView = lazy(() =>
  import("../terminal/TerminalView").then(({ TerminalView }) => ({ default: TerminalView })),
);

// Terminals stay mounted behind other sections so output keeps flowing.
// Each session uses its host's appearance.
export function TerminalScreen({
  visible,
  terminalSessions,
  activeSessionId,
  splitView,
  settings,
  hostAppearance,
  hostVPN,
  api,
  runTargets,
  onOpenRemotePath,
  onOSC52Change,
  onOpenNavigation,
}: {
  // onOpenNavigation opens the sidebar from a phone's terminal top bar; undefined while the sidebar stays open.
  onOpenNavigation?: (() => void) | undefined;
  visible: boolean;
  terminalSessions: TerminalSessionsState;
  activeSessionId: string | null;
  splitView: SplitViewController;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
  // hostVPN は、alias ごとに通るVPN経路の名前である。
  hostVPN: Map<string, string>;
  // api is the terminal API of the run target the sessions belong to.
  api: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath" | "terminalSessionDirectory">;
  // runTargets says which engine that is; its selected alias is null for this
  // machine.
  runTargets: RunTargetsState;
  onOpenRemotePath: (
    alias: string,
    path: string,
    action: RemotePathAction,
  ) => void;
  // onOSC52Change turns OSC 52 on or off for every terminal.
  onOSC52Change: (enabled: boolean) => Promise<void>;
}) {
  const runTarget = runTargets.selected;
  // Only an engine on an SSH host refuses a list; this machine's own list
  // keeps failing quietly.
  const refusal = runTarget === null ? null : terminalSessions.remoteRefusal;
  const [splitRequest, setSplitRequest] = useState<{ targetSessionId: string; edge: PaneEdge } | null>(null);
  const session = terminalSessions.sessions.find((session) => session.id === activeSessionId) ?? null;
  const keepScreenAwake = useKeepScreenAwake();
  // Only while a terminal is on the screen: other sections let the screen turn off as usual.
  useScreenWakeLock(keepScreenAwake && visible && session !== null);
  useEffect(() => {
    document.title = session === null ? "sshc" : `${session.presentation?.displayTitle ?? session.title} · sshc`;
    return () => { document.title = "sshc"; };
  }, [session]);
  function renderTerminal(session: TerminalSession, headerActions: ReactNode, inPane: boolean) {
    const appearance = resolveAppearance(
      session.alias === undefined
        ? undefined
        : hostAppearance.get(session.alias),
      settings.appearance,
    );
    const vpnProfile = session.alias === undefined ? "" : hostVPN.get(session.alias) ?? "";
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <Suspense fallback={<RouteSkeleton kind="terminal" />}>
        <TerminalView
          key={session.id}
          session={session}
          active={visible && activeSessionId === session.id}
          inPane={inPane}
          focusRequest={splitView.state.focusRevision}
          headerActions={headerActions}
          {...(splitView.compact ? {} : { onSplit: (edge: PaneEdge) => setSplitRequest({ targetSessionId: session.id, edge }) })}
          canSplit={terminalSessions.sessions.some((candidate) => splitView.canSplit(candidate.id, session.id))}
          {...(splitView.groupBySession.has(session.id) ? { onRemoveFromSplit: () => splitView.removeFromSplit(session.id) } : {})}
          api={api}
          {...(runTarget === null ? {} : { runTarget })}
          searchShortcutActive={visible && activeSessionId === session.id}
          {...(settings.fontSize === undefined
            ? {}
            : { fontSize: settings.fontSize })}
          {...(settings.browserScrollbackLines === undefined
            ? {}
            : { scrollbackLines: settings.browserScrollbackLines })}
          osc52Enabled={settings.osc52 ?? false}
          vpnProfile={vpnProfile}
          jisYenBackslash={settings.jisYenBackslash ?? false}
          composerEnterSends={settings.composerEnterSends ?? false}
          ligatures={settings.ligatures ?? false}
          statusLine={settings.statusLine ?? true}
          commandMarks={settings.commandMarks ?? true}
          stickyCommand={settings.stickyCommand ?? true}
          commandScrollbarMarks={settings.commandScrollbarMarks ?? true}
          onOsc52Change={onOSC52Change}
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
          onExit={(exit) => terminalSessions.markExited(session.id, exit)}
          onReconnect={() => terminalSessions.reconnect(session.id)}
          onStopReconnect={() => terminalSessions.stopReconnect(session.id)}
          onCloseSession={() => void terminalSessions.close(session.id)}
          onOpenRemotePath={onOpenRemotePath}
          onOpenNavigation={onOpenNavigation}
        />
        </Suspense>
      </div>
    );
  }
  return <div className="relative h-full">
    {/* What the refusing engine listed before cannot be used, so it is kept out of reach. */}
    <div className="h-full" inert={refusal !== null}>
      <TerminalSplitView splitView={splitView} sessions={terminalSessions.sessions} retainedTerminals={settings.retainedTerminals ?? defaultRetainedTerminals} renderTerminal={renderTerminal} />
    </div>
    {runTarget !== null && refusal === null && terminalSessions.opening ? <RunTargetConnectingPanel alias={runTarget} /> : null}
    {runTarget === null || refusal === null ? null : <RunTargetRefusalPanel
      alias={runTarget}
      refusal={refusal}
      refusedAt={terminalSessions.remoteRefusalAt}
      api={runTargets.api}
      localVersion={runTargets.localVersion}
      vaultRevision={runTargets.vaultUnlock.revision}
      onReconnect={terminalSessions.refresh}
      onUseThisMachine={() => runTargets.select(null)}
      onUnlocked={runTargets.vaultUnlock.unlocked}
      onOpenInNewTab={() => void runTargets.openInNewTab(runTarget)}
      onOpenEngineInfo={() => runTargets.engineInfo.ask(runTarget)} />}
    {splitRequest === null ? null : <SplitSessionPicker
      sessions={terminalSessions.sessions.filter((candidate) => splitView.canSplit(candidate.id, splitRequest.targetSessionId))}
      onClose={() => setSplitRequest(null)}
      onSelect={(sourceSessionId) => {
        splitView.split({ ...splitRequest, sourceSessionId });
        setSplitRequest(null);
      }} />}
  </div>;
}
