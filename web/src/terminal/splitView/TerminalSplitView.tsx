import type { ReactNode } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import { shortcutsBlocked } from "../../keyconfig/bindings";
import { SplitResizeHandle } from "../../ui/SplitResizeHandle";
import { useTouchTerminal } from "../touchMode";
import { terminalDisplayTitle } from "../terminalPresentation";
import { TerminalEmptyState } from "../TerminalEmptyState";
import { splitViewGeometry } from "./geometry";
import { maximumSplitViewPanes } from "./layout";
import { maximumSplitGroups } from "./splitGroups";
import { PaneToolbar } from "./PaneToolbar";
import { useRetainedSessions } from "./retainedSessions";
import { usePaneDrag } from "./usePaneDrag";
import type { SplitViewController } from "./useSplitView";

const previewClass = {
  left: "inset-y-0 left-0 w-1/2",
  right: "inset-y-0 right-0 w-1/2",
  top: "inset-x-0 top-0 h-1/2",
  bottom: "inset-x-0 bottom-0 h-1/2",
};

export function TerminalSplitView({ splitView, sessions, renderTerminal }: {
  splitView: SplitViewController;
  sessions: TerminalSession[];
  renderTerminal: (session: TerminalSession, headerActions: ReactNode) => ReactNode;
}) {
  const t = useTranslate();
  const { state, compact, dispatch, selectSession } = splitView;
  const { panes, dividers } = splitViewGeometry(state.root);
  const drag = usePaneDrag(splitView);
  const touchTerminal = useTouchTerminal();
  const multiple = panes.length > 1;
  const singleVisible = compact || state.focusMode;
  const sessionById = new Map(sessions.map((session) => [session.id, session]));
  const retained = useRetainedSessions(panes.map((pane) => pane.sessionId), new Set(sessionById.keys()));
  const selected = state.activeSessionId;
  function dropHint(swapping: boolean): string {
    if (swapping) return t("splitView.swap");
    if (drag.atLimit) return t("splitView.maxPanes", { count: maximumSplitViewPanes });
    if (drag.atGroupLimit) return t("splitView.maxGroups", { count: maximumSplitGroups });
    return t("splitView.drop");
  }
  if (panes.length === 0) return <TerminalEmptyState />;
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col" onKeyDownCapture={(event) => {
      if (shortcutsBlocked(event.nativeEvent)) return;
      if (event.key === "Escape" && !(event.target instanceof HTMLInputElement)) {
        drag.clear();
        if (state.focusMode) {
          event.preventDefault();
          event.stopPropagation();
          dispatch({ type: "focus-mode", enabled: false });
        }
      }
      if (!multiple || !event.ctrlKey || !event.altKey || event.shiftKey || event.metaKey) return;
      const step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
      if (step === 0) return;
      event.preventDefault();
      event.stopPropagation();
      const index = panes.findIndex((pane) => pane.sessionId === selected);
      selectSession(panes[(index + step + panes.length) % panes.length]!.sessionId);
    }}>
      {compact && multiple ? (
        <nav aria-label={t("splitView.panes")} className="flex shrink-0 gap-1 overflow-x-auto border-b border-line bg-toolbar p-1">
          {panes.map(({ sessionId }) => {
            const session = sessionById.get(sessionId);
            return session === undefined ? null : (
              <button key={sessionId} type="button" aria-current={selected === sessionId ? "true" : undefined} className={`min-h-11 shrink-0 rounded px-3 text-sm ${selected === sessionId ? "bg-select-fill text-accent" : "text-ink-muted"}`} onClick={() => selectSession(sessionId)}>{terminalDisplayTitle(session)}</button>
            );
          })}
        </nav>
      ) : null}
      {/* The panes are placed absolutely, so the outer half of the 8px gap is this wrapper's padding. */}
      <div className={`flex min-h-0 min-w-0 flex-1 ${compact ? "" : "p-1"}`}>
        <div className="relative min-h-0 min-w-0 flex-1" data-split-view>
          {/* Moving the drag source to make room for this hint interrupts native dragging. */}
          {drag.moving === null ? null : (
            <p role="status" className="pointer-events-none absolute inset-x-2 bottom-2 z-30 rounded bg-toolbar px-2 py-1 text-xs text-ink-muted">
              {t("splitView.swapHint")}
            </p>
          )}
          {/* The kept sessions are drawn by the same code under the same parent and key as the panes, so a
              session moving between the two keeps its terminal rather than being built anew. */}
          {[...panes.map((pane) => ({ ...pane, kept: false })), ...retained.map((sessionId) => ({ sessionId, style: undefined, kept: true }))].map(({ sessionId, style, kept }) => {
            const session = sessionById.get(sessionId);
            if (session === undefined) return null;
            const active = !kept && selected === sessionId;
            const hidden = kept || (singleVisible && !active);
            const target = drag.target?.sessionId === sessionId ? drag.target : null;
            const title = terminalDisplayTitle(session);
            const headerActions = multiple && !compact && !kept ? (
              <PaneToolbar
                title={title}
                moving={drag.moving === sessionId}
                focusMode={state.focusMode}
                onSwap={() => drag.pick(sessionId)}
                onDragStart={(event) => drag.begin(event, sessionId)}
                onDragEnd={drag.clear}
                onFocusMode={() => {
                  selectSession(sessionId);
                  dispatch({ type: "focus-mode", enabled: !state.focusMode });
                }}
                onClose={() => dispatch({ type: "close", sessionId })}
              />
            ) : null;
            return (
              <div key={sessionId} data-split-view-pane={kept ? undefined : sessionId} data-split-view-kept={kept ? sessionId : undefined}
                data-pane-active={active} hidden={hidden}
                style={singleVisible || kept ? { inset: 0 } : style}
                className={`absolute min-h-0 min-w-0 flex-col ${compact ? "" : "p-1"} ${hidden ? "hidden" : "flex"}`}
                onPointerDown={() => selectSession(sessionId, false)}
                onFocusCapture={() => selectSession(sessionId, false)}
                onClick={(event) => {
                  // A click on the header's title leaves the keyboard on the page body, so the pane would look
                  // chosen while typing goes nowhere. A click that put the focus in the pane (the terminal, a
                  // button) needs nothing, nor does one in a dialog the pane opened elsewhere on the page.
                  // A touch screen keeps its keyboard closed until the terminal itself is tapped.
                  const pane = event.currentTarget;
                  if (compact || touchTerminal) return;
                  if (!pane.contains(event.target as Node) || pane.contains(document.activeElement)) return;
                  selectSession(sessionId);
                }}
                onDragEnter={(event) => drag.dragOver(event, sessionId)}
                onDragOver={(event) => drag.dragOver(event, sessionId)}
                onDragLeave={drag.leave}
                onDrop={(event) => drag.drop(event, sessionId)}>
                <div data-split-view-board className={`relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden ${compact ? "" : `rounded-pane border shadow-pane ${multiple && active ? "border-accent/45" : "border-line"}`}`}>
                  {renderTerminal(session, headerActions)}
                  {target === null || compact ? null : (
                    <div data-pane-drop-preview={target.edge ?? "swap"} className={`pointer-events-none absolute z-20 grid place-items-center border-2 border-accent bg-accent/20 ${target.edge === null ? "inset-0" : previewClass[target.edge]}`}>
                      <span className="rounded bg-card px-2 py-1 text-xs text-ink" role="status">{dropHint(target.edge === null)}</span>
                    </div>
                  )}
                </div>
              </div>
            );
          })}
          {singleVisible ? null : dividers.map(({ path, style, direction, ratio, minimum }) => (
            <div key={path.join("/")} className="pointer-events-none absolute" style={style}>
              <SplitResizeHandle direction={direction} ratio={ratio} minimum={minimum} label={t("splitView.resize")} look="gap"
                style={direction === "horizontal" ? { position: "absolute", pointerEvents: "auto", left: `${ratio}%`, top: 0, height: "100%", transform: "translateX(-50%)" } : { position: "absolute", pointerEvents: "auto", top: `${ratio}%`, left: 0, width: "100%", transform: "translateY(-50%)" }}
                onRatioChange={(next) => dispatch({ type: "resize", path, ratio: next })} />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
