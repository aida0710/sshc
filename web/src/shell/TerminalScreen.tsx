import { Suspense, lazy, useEffect, useRef, type ReactNode } from "react";
import type { TerminalAppearance, TerminalSettings } from "../api/settings";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { resolveAppearance } from "../terminal/appearance";
import { TerminalSplitView } from "../terminal/splitView/TerminalSplitView";
import { defaultRetainedTerminals } from "../terminal/splitView/retainedSessions";
import type { SplitViewController } from "../terminal/splitView/useSplitView";
import type { RemotePathAction } from "../sftp/SFTPPanel";
import { hostSettingsToDrawWith, type HostSettings } from "../terminal/hostSettings";
import { SessionEngineContext } from "../terminal/sessionDestination";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { ExitReport } from "../terminal/stream";
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

// ShownTerminals is what a run target's terminals are drawn from: its sessions, how they are split and its API.
type ShownTerminals = {
  runTarget: string | null;
  sessions: TerminalSession[];
  splitView: SplitViewController;
  api: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath" | "terminalSessionDirectory">;
} & HostSettings;

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
  hostSettingsRead,
  api,
  runTargets,
  runTargetLink,
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
  // hostSettingsRead は、hostAppearance と hostVPN が、今の実行先の SSH の設定から読んだものかどうか。
  // 実行先を替えた直後は、読み終えるまで空である。
  hostSettingsRead: boolean;
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
  function splitRefusalText(view: SplitViewController, sessionId: string): string | undefined {
    switch (view.splitRefusal(sessionId)) {
      case "ended": return t("splitView.refusedEnded");
      case "groups": return t("splitView.maxGroups", { count: maximumSplitGroups });
      case "sessions": return t("terminal.limitReached", { max: terminalSessions.maxSessions });
      case null: return undefined;
    }
  }
  // terminalRenderer draws the terminals of one run target: the one shown, or one kept behind it (shownTerminals).
  // A kept terminal is not on the screen whatever the section, and what its stream says about its session is left for
  // the list to say when its run target is shown again: the list this screen holds is another engine's.
  const terminalRenderer = (shown: ShownTerminals, onScreen: boolean) => function renderTerminal(session: TerminalSession, headerActions: ReactNode, inPane: boolean) {
    const appearance = resolveAppearance(
      session.alias === undefined
        ? undefined
        : shown.hostAppearance.get(session.alias),
      settings.appearance,
    );
    const vpnProfile = session.alias === undefined ? "" : shown.hostVPN.get(session.alias) ?? "";
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <Suspense fallback={<RouteSkeleton kind="terminal" />}>
        <TerminalView
          key={session.id}
          session={session}
          active={onScreen && visible && activeSessionId === session.id}
          inPane={inPane}
          focusRequest={shown.splitView.state.focusRevision}
          headerActions={headerActions}
          {...(shown.splitView.compact ? {} : { onSplit: (edge: PaneEdge) => shown.splitView.requestSplit({ targetSessionId: session.id, edge }) })}
          splitRefusal={splitRefusalText(shown.splitView, session.id)}
          {...(shown.splitView.groupBySession.has(session.id) ? { onRemoveFromSplit: () => shown.splitView.removeFromSplit(session.id) } : {})}
          api={shown.api}
          {...(shown.runTarget === null ? {} : { runTarget: shown.runTarget })}
          searchShortcutActive={onScreen && visible && activeSessionId === session.id}
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
          {...(onScreen ? { onForwardsChanged: terminalSessions.refresh } : {})}
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
          {...(onScreen ? { onExit: (exit: ExitReport) => terminalSessions.markExited(session.id, exit), onProgramStateChanged: terminalSessions.programStateChanged } : {})}
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
  };

  // The terminals of a run target stay mounted while another is shown, so that coming back finds their scrollback,
  // the size the text was zoomed to and what was being typed. A run target's own list is read again when it is shown;
  // until it answers, and while it is behind another, its terminals are drawn from what it listed last.
  const kept = useRef(new Map<string | null, ShownTerminals>()).current;
  const listed = terminalSessions.loaded && refusal === null;
  if (listed) {
    // A terminal draws with the appearance its connection asks for. Those settings are read after the engine is
    // chosen, and are another engine's while it is behind: until this engine's are read, its terminals keep the ones
    // they were drawn with. A terminal given another font changes its size, and the program in it is told.
    const hostSettings = hostSettingsToDrawWith({ current: { hostAppearance, hostVPN }, read: hostSettingsRead, drawnWith: kept.get(runTarget) });
    kept.set(runTarget, { runTarget, sessions: terminalSessions.sessions, splitView, api, ...hostSettings });
  }
  for (const keptRunTarget of kept.keys()) {
    if (keptRunTarget !== null && keptRunTarget !== runTarget && !runTargets.engines.includes(keptRunTarget)) kept.delete(keptRunTarget);
  }
  const retainedTerminals = settings.retainedTerminals ?? defaultRetainedTerminals;
  return <div className="relative h-full">
    {/* One list, keyed by run target: a run target's terminals keep their place in the tree when it goes behind
        another and when it comes back, so React does not mount them again. */}
    {[...kept.values()].map((shown) => shown.runTarget === runTarget ? (
      // What the refusing engine listed before cannot be used, so it is kept out of reach, and out of sight: the panel
      // ends above a phone's floating bottom bar so the bar stays usable, and the terminal's key bar and composer
      // would show around the bar. Hidden, not removed, the terminals keep their size and output for when the engine
      // answers again.
      <div key={runTarget ?? ""} className={refusal === null ? "h-full" : "invisible h-full"} inert={refusal !== null}>
        <SessionEngineContext.Provider value={shown.runTarget}>
          <TerminalSplitView splitView={shown.splitView} sessions={shown.sessions} retainedTerminals={retainedTerminals} renderTerminal={terminalRenderer(shown, listed)} />
        </SessionEngineContext.Provider>
      </div>
    ) : (
      <div key={shown.runTarget ?? ""} data-kept-run-target={shown.runTarget ?? ""} aria-hidden="true" inert className="invisible absolute inset-0">
        <SessionEngineContext.Provider value={shown.runTarget}>
          <TerminalSplitView splitView={shown.splitView} sessions={shown.sessions} retainedTerminals={retainedTerminals} renderTerminal={terminalRenderer(shown, false)} behind />
        </SessionEngineContext.Provider>
      </div>
    ))}
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
