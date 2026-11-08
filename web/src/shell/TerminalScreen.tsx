import { Suspense, lazy, useEffect, type ReactNode } from "react";
import type { TerminalAppearance, TerminalSettings } from "../api/settings";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { resolveAppearance } from "../terminal/appearance";
import { TerminalEmptyState } from "../terminal/TerminalEmptyState";
import { TerminalSplitView } from "../terminal/splitView/TerminalSplitView";
import { defaultRetainedTerminals } from "../terminal/splitView/retainedSessions";
import type { SplitViewController } from "../terminal/splitView/useSplitView";
import type { RemotePathAction } from "../sftp/SFTPPanel";
import type { TerminalSessionsState } from "../terminal/sessions";
import { RouteSkeleton } from "../ui/RouteSkeleton";
import type { PaneEdge } from "../terminal/splitView/layout";
import { maximumSplitGroups } from "../terminal/splitView/splitGroups";
import { RunTargetConnectingPanel } from "../terminal/RunTargetConnectingPanel";
import { RunTargetRefusalPanel } from "../terminal/RunTargetRefusalPanel";
import type { RunTargetLink } from "../terminal/useRunTargetLink";
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
  runTargetLink,
  onOpenRemotePath,
  onOSC52Change,
  onOpenNavigation,
  onOpenLocalShell,
  onAddConnection,
  onOpenSync,
  onOpenConnection,
  onShowConnections,
}: {
  onOpenLocalShell: () => void;
  onAddConnection: () => void;
  onOpenSync: () => void;
  // onOpenConnection opens a session to a connection the empty state lists as used recently.
  onOpenConnection: (alias: string) => void;
  onShowConnections: () => void;
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
  runTargetLink: RunTargetLink;
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
  // Opening a session on this machine shows nothing here: it takes no connection, and a failure stays on the screen it
  // was asked from. Reading its sessions is waited for like a run target's.
  const waitingForEngine = refusal === null && (!terminalSessions.loaded || (runTarget !== null && terminalSessions.opening));
  const t = useTranslate();
  const session = terminalSessions.sessions.find((session) => session.id === activeSessionId) ?? null;
  const keepScreenAwake = useKeepScreenAwake();
  // Only while a terminal is on the screen: other sections let the screen turn off as usual.
  useScreenWakeLock(keepScreenAwake && visible && session !== null);
  useEffect(() => {
    document.title = session === null ? "sshc" : `${session.presentation?.displayTitle ?? session.title} · sshc`;
    return () => { document.title = "sshc"; };
  }, [session]);
  // Why the terminal's 「…」 cannot split beside a session, for its disabled items; undefined when it can.
  function splitRefusalText(sessionId: string): string | undefined {
    switch (splitView.splitRefusal(sessionId)) {
      case "ended": return t("splitView.refusedEnded");
      case "groups": return t("splitView.maxGroups", { count: maximumSplitGroups });
      case "sessions": return t("terminal.limitReached", { max: terminalSessions.maxSessions });
      case null: return undefined;
    }
  }
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
          {...(splitView.compact ? {} : { onSplit: (edge: PaneEdge) => splitView.requestSplit({ targetSessionId: session.id, edge }) })}
          splitRefusal={splitRefusalText(session.id)}
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
          onProgramProgressChanged={terminalSessions.programProgressChanged}
          onReconnect={() => terminalSessions.reconnect(session.id)}
          onStopReconnect={() => terminalSessions.stopReconnect(session.id)}
          onCloseSession={() => void terminalSessions.close(session.id)}
          onAnswerHostKey={(fingerprint, accept) => terminalSessions.answerHostKey(session.id, fingerprint, accept)}
          onAnswerPasswordSave={(save) => terminalSessions.answerPasswordSave(session.id, save)}
          onOpenRemotePath={onOpenRemotePath}
          onOpenNavigation={onOpenNavigation}
        />
        </Suspense>
      </div>
    );
  }
  return <div className="relative h-full">
    {/* What the refusing engine listed before cannot be used, so it is kept out of reach, and out of sight: the panel
        ends above a phone's floating bottom bar so the bar stays usable, and the terminal's key bar and composer
        would show around the bar. Hidden, not removed, the terminals keep their size and output for when the engine
        answers again. */}
    <div className={refusal === null ? "h-full" : "invisible h-full"} inert={refusal !== null}>
      <TerminalSplitView splitView={splitView} sessions={terminalSessions.sessions} retainedTerminals={settings.retainedTerminals ?? defaultRetainedTerminals} renderTerminal={renderTerminal}
        // Until the engine answers, it is not known to have no sessions.
        emptyState={terminalSessions.loaded ? <TerminalEmptyState runTarget={runTarget} visible={visible} onOpenLocalShell={onOpenLocalShell} onAddConnection={onAddConnection} onOpenSync={onOpenSync} onOpenConnection={onOpenConnection} onShowConnections={onShowConnections} /> : null}
      />
    </div>
    {waitingForEngine ? (
      <RunTargetConnectingPanel
        key={runTarget ?? ""}
        alias={runTarget}
        awaitingEngine={runTarget !== null && runTargetLink.connectedEngines.has(runTarget)}
        onUseThisMachine={() => runTargets.select(null)}
      />
    ) : null}
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
  </div>;
}
