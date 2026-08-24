import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import type { TerminalSession } from "../../api/integrations";
import { failureCode } from "../../api/client";
import { useTranslate } from "../../i18n/context";
import { Button } from "../../ui/surface";
import { BrandMark } from "../../ui/BrandMark";
import { executionTargets, paneIDs, reduceLayout, restoreLayout, storeLayout, type ExecutionTarget, type LayoutAction, type LayoutState, type RuntimeNode, type RuntimePane, type SplitDirection, type StoredNode } from "./layout";
import { workspaceApi, type SavedWorkspace } from "./api";
import { WorkspaceCommandCenter } from "./WorkspaceCommandCenter";

export type BroadcastInput = { sequence: number; source: string; data: string };
export type WorkspaceRestoreRequest = { id: string; sequence: number };

function paneID(): string { return crypto.randomUUID().replaceAll("-", ""); }

function visit(root: StoredNode, callback: (id: string, alias: string) => void) {
  if (root.pane !== undefined) { callback(root.pane.id, root.pane.alias); return; }
  visit(root.split.first, callback); visit(root.split.second, callback);
}

function findPane(root: RuntimeNode, id: string): RuntimePane | null {
  if (root.pane !== undefined) return root.pane.id === id ? root.pane : null;
  return findPane(root.split.first, id) ?? findPane(root.split.second, id);
}

export function TerminalWorkspace({
  sessions, activeSessionId, onActive, onOpenAlias, renderTerminal, restoreRequest = null, onRestoreConsumed = () => undefined,
}: {
  sessions: TerminalSession[];
  activeSessionId: string | null;
  onActive: (id: string) => void;
  onOpenAlias: (alias: string) => Promise<TerminalSession | null>;
  renderTerminal: (session: TerminalSession, onInput: (data: string) => void, injected: BroadcastInput | null) => ReactNode;
  restoreRequest?: WorkspaceRestoreRequest | null;
  onRestoreConsumed?: (sequence: number) => void;
}) {
  const t = useTranslate();
  const [layout, setLayout] = useState<LayoutState | null>(null);
  const [saved, setSaved] = useState<SavedWorkspace[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [broadcast, setBroadcast] = useState(false);
  const [commandCenter, setCommandCenter] = useState(false);
  const [focusModePaneId, setFocusModePaneId] = useState<string | null>(null);
  const [movingPaneId, setMovingPaneId] = useState<string | null>(null);
  const [dropPaneId, setDropPaneId] = useState<string | null>(null);
  const [input, setInput] = useState<BroadcastInput | null>(null);
  const [problem, setProblem] = useState("");
  const consumedRestore = useRef(0);
  const active = sessions.find((session) => session.id === activeSessionId) ?? null;
  const commandTargets = useMemo<ExecutionTarget[]>(() => {
    if (layout !== null) return executionTargets(layout.root, "pane");
    if (active?.kind !== "ssh" || active.alias === undefined) return [];
    return [{ targetId: active.id, alias: active.alias, state: "connected" }];
  }, [active, layout]);

  const sessionByID = useMemo(() => new Map(sessions.map((session) => [session.id, session])), [sessions]);
  useEffect(() => { void workspaceApi.list().then(setSaved).catch(() => undefined); }, []);
  useEffect(() => {
    if (movingPaneId === null) return;
    if (layout === null || !paneIDs(layout.root).includes(movingPaneId)) {
      setMovingPaneId(null);
      setDropPaneId(null);
    }
  }, [layout, movingPaneId]);
  useEffect(() => {
    if (focusModePaneId === null) return;
    if (layout === null || !paneIDs(layout.root).includes(focusModePaneId)) setFocusModePaneId(null);
  }, [focusModePaneId, layout]);
  useEffect(() => {
    const exit = (event: KeyboardEvent) => { if (event.key === "Escape") setFocusModePaneId(null); };
    window.addEventListener("keydown", exit);
    return () => window.removeEventListener("keydown", exit);
  }, []);

  function update(action: LayoutAction) { setLayout((current) => current === null ? current : reduceLayout(current, action)); }

  function finishPaneMove(sourcePaneId: string, targetPaneId: string) {
    if (sourcePaneId !== targetPaneId) update({ type: "swap-panes", sourcePaneId, targetPaneId });
    setMovingPaneId(null);
    setDropPaneId(null);
  }

  function choosePaneMove(paneId: string) {
    if (movingPaneId === null) {
      setMovingPaneId(paneId);
      return;
    }
    finishPaneMove(movingPaneId, paneId);
  }

  function beginPaneDrag(event: DragEvent<HTMLButtonElement>, paneId: string) {
    event.stopPropagation();
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", paneId);
    setMovingPaneId(paneId);
  }

  function dropPane(event: DragEvent<HTMLDivElement>, targetPaneId: string) {
    event.preventDefault();
    event.stopPropagation();
    const sourcePaneId = movingPaneId ?? event.dataTransfer.getData("text/plain");
    if (sourcePaneId !== "") finishPaneMove(sourcePaneId, targetPaneId);
  }

  function beginResize(event: ReactPointerEvent<HTMLDivElement>, path: ("first" | "second")[], direction: SplitDirection) {
    event.preventDefault();
    event.stopPropagation();
    const container = event.currentTarget.parentElement;
    if (container === null) return;
    const pointerId = event.pointerId;
    event.currentTarget.setPointerCapture(pointerId);
    const move = (next: PointerEvent) => {
      const bounds = container.getBoundingClientRect();
      const extent = direction === "horizontal" ? bounds.width : bounds.height;
      if (extent <= 0) return;
      const offset = direction === "horizontal" ? next.clientX - bounds.left : next.clientY - bounds.top;
      update({ type: "resize-split", path, ratio: offset / extent * 100 });
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop, { once: true });
    window.addEventListener("pointercancel", stop, { once: true });
  }

  async function split(direction: SplitDirection) {
    const focusedPane = layout === null ? null : findPane(layout.root, layout.focusedPaneId);
    const focusedSession = layout === null
      ? active
      : focusedPane?.sessionId === undefined ? null : sessionByID.get(focusedPane.sessionId) ?? null;
    if (focusedSession?.kind !== "ssh" || focusedSession.alias === undefined) return;
    const firstID = layout?.focusedPaneId ?? paneID();
    if (layout === null) setLayout(restoreLayout({ pane: { id: firstID, alias: focusedSession.alias } }, firstID));
    const secondID = paneID();
    const opened = await onOpenAlias(focusedSession.alias);
    if (opened === null) { setProblem(t("workspace.openFailed")); return; }
    setLayout((current) => {
      const base = current ?? restoreLayout({ pane: { id: firstID, alias: focusedSession.alias ?? "" } }, firstID);
      let next = reduceLayout(base, { type: "connection-started", paneId: firstID, sessionId: focusedSession.id });
      next = reduceLayout(next, { type: "split", paneId: firstID, direction, pane: { id: secondID, alias: focusedSession.alias ?? "" } });
      return reduceLayout(next, { type: "connection-started", paneId: secondID, sessionId: opened.id });
    });
    onActive(opened.id);
  }

  async function saveWorkspace() {
    let effective = layout;
    if (effective === null && active?.kind === "ssh" && active.alias !== undefined) {
      const id = paneID();
      effective = reduceLayout(restoreLayout({ pane: { id, alias: active.alias } }, id), { type: "connection-started", paneId: id, sessionId: active.id });
      setLayout(effective);
    }
    if (effective === null) return;
    const name = window.prompt(t("workspace.namePrompt"), saved.find((item) => item.id === selectedWorkspace)?.name ?? "")?.trim() ?? "";
    if (name === "") return;
    try {
      const stored = storeLayout(effective);
      const value = selectedWorkspace === "" ? await workspaceApi.create({ name, ...stored }) : await workspaceApi.update(selectedWorkspace, { name, ...stored });
      setSelectedWorkspace(value.id); setSaved(await workspaceApi.list()); setProblem("");
    } catch (error) { setProblem(failureCode(error) || "workspace_failed"); }
  }

  const restoreWorkspace = useCallback(async (id: string) => {
    if (id === "") return;
    try {
      const stored = await workspaceApi.restore(id);
      setSelectedWorkspace(id);
      setFocusModePaneId(null);
      let restored = restoreLayout(stored.layout, stored.focusedPaneId);
      const panes: { id: string; alias: string }[] = [];
      visit(stored.layout, (paneId, alias) => {
        panes.push({ id: paneId, alias });
        restored = reduceLayout(restored, { type: "connection-starting", paneId });
      });
      setLayout(restored);
      await Promise.all(panes.map(async (pane) => {
        const session = await onOpenAlias(pane.alias);
        if (session === null) {
          setLayout((current) => current === null ? current : reduceLayout(current, {
            type: "connection-failed", paneId: pane.id, problem: "open_failed",
          }));
          return;
        }
        setLayout((current) => current === null ? current : reduceLayout(current, {
          type: "connection-started", paneId: pane.id, sessionId: session.id,
        }));
        if (pane.id === stored.focusedPaneId) onActive(session.id);
      }));
      setProblem("");
    } catch (error) { setProblem(failureCode(error) || "workspace_failed"); }
  }, [onActive, onOpenAlias]);

  useEffect(() => {
    if (restoreRequest === null || restoreRequest.sequence <= consumedRestore.current) return;
    consumedRestore.current = restoreRequest.sequence;
    onRestoreConsumed(restoreRequest.sequence);
    void restoreWorkspace(restoreRequest.id);
  }, [onRestoreConsumed, restoreRequest, restoreWorkspace]);

  function terminal(session: TerminalSession) {
    return renderTerminal(session, (data) => {
      if (broadcast) setInput((current) => ({ sequence: (current?.sequence ?? 0) + 1, source: session.id, data }));
    }, input?.source === session.id ? null : input);
  }

  function renderNode(node: RuntimeNode, path: ("first" | "second")[] = []): ReactNode {
    if (node.pane !== undefined) {
      const session = node.pane.sessionId === undefined ? undefined : sessionByID.get(node.pane.sessionId);
      const multiple = layout !== null && paneIDs(layout.root).length > 1;
      const moving = movingPaneId === node.pane.id;
      const dropping = dropPaneId === node.pane.id && !moving;
      return (
        <div
          key={node.pane.id}
          data-workspace-pane={node.pane.id}
          data-pane-alias={node.pane.alias}
          className={`relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden border ${layout?.focusedPaneId === node.pane.id ? "border-accent" : "border-line"} ${moving ? "ring-2 ring-accent" : dropping ? "ring-2 ring-live" : ""}`}
          onPointerDown={() => { update({ type: "focus", paneId: node.pane.id }); if (session !== undefined) onActive(session.id); }}
          onDragEnter={(event) => { if (movingPaneId !== null && movingPaneId !== node.pane.id) { event.preventDefault(); setDropPaneId(node.pane.id); } }}
          onDragOver={(event) => { if (movingPaneId !== null && movingPaneId !== node.pane.id) { event.preventDefault(); event.dataTransfer.dropEffect = "move"; } }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropPaneId((current) => current === node.pane.id ? null : current); }}
          onDrop={(event) => dropPane(event, node.pane.id)}
        >
          {session === undefined ? <div className="grid h-full min-h-40 place-items-center text-sm text-ink-muted">{node.pane.state === "failed" ? node.pane.problem : t("workspace.reconnecting")}</div> : terminal(session)}
          {multiple ? <button type="button" draggable aria-pressed={moving} aria-label={t(moving ? "workspace.movePanePicked" : "workspace.movePane", { alias: node.pane.alias })} title={t("workspace.movePane", { alias: node.pane.alias })} className="absolute left-2 top-2 z-20 cursor-grab rounded bg-toolbar px-2 py-1 text-xs shadow-sm active:cursor-grabbing" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); choosePaneMove(node.pane.id); }} onDragStart={(event) => beginPaneDrag(event, node.pane.id)} onDragEnd={() => { setMovingPaneId(null); setDropPaneId(null); }} onKeyDown={(event) => { if (event.key === "Escape") { setMovingPaneId(null); setDropPaneId(null); } }}>⠿</button> : null}
          {multiple ? <button type="button" aria-pressed={focusModePaneId === node.pane.id} aria-label={t(focusModePaneId === node.pane.id ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: node.pane.alias })} title={t(focusModePaneId === node.pane.id ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: node.pane.alias })} className="absolute right-9 top-2 z-20 rounded bg-toolbar px-1.5 text-xs" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); setFocusModePaneId((current) => current === node.pane.id ? null : node.pane.id); }}>⛶</button> : null}
          {multiple ? <button type="button" aria-label={t("workspace.closePane")} className="absolute right-2 top-2 z-20 rounded bg-toolbar px-1.5 text-xs" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); update({ type: "close", paneId: node.pane.id }); }}>×</button> : null}
          {dropping ? <div className="pointer-events-none absolute inset-0 z-10 grid place-items-center bg-accent/10"><span className="rounded bg-toolbar px-3 py-2 text-xs font-medium shadow">{t("workspace.swapWith", { alias: node.pane.alias })}</span></div> : null}
        </div>
      );
    }
    const row = node.split.direction === "horizontal";
    const resizeStep = (event: ReactKeyboardEvent<HTMLDivElement>) => {
      const decrease = event.key === (row ? "ArrowLeft" : "ArrowUp");
      const increase = event.key === (row ? "ArrowRight" : "ArrowDown");
      if (!decrease && !increase) return;
      event.preventDefault();
      update({ type: "resize-split", path, ratio: node.split.ratio + (decrease ? -5 : 5) });
    };
    return <div className={`flex h-full min-h-0 min-w-0 flex-1 ${row ? "flex-row" : "flex-col"}`}><div style={{ flexBasis: `${node.split.ratio}%` }} className="flex min-h-0 min-w-0">{renderNode(node.split.first, [...path, "first"])}</div><div role="separator" tabIndex={0} aria-label={t("workspace.resizeSplit")} aria-orientation={row ? "vertical" : "horizontal"} aria-valuemin={10} aria-valuemax={90} aria-valuenow={node.split.ratio} onPointerDown={(event) => beginResize(event, path, node.split.direction)} onKeyDown={resizeStep} className={`shrink-0 touch-none bg-line transition-colors hover:bg-accent focus:bg-accent focus:outline-none ${row ? "w-1 cursor-col-resize" : "h-1 cursor-row-resize"}`} /><div style={{ flexBasis: `${100 - node.split.ratio}%` }} className="flex min-h-0 min-w-0">{renderNode(node.split.second, [...path, "second"])}</div></div>;
  }

  // A local shell does not participate in an SSH workspace. Keep its terminal
  // surface identical to the single-console path so fitting and selection are
  // unaffected by controls that cannot apply to it.
  if (layout === null && active?.kind === "shell") {
    return <div className="flex h-full min-h-0 flex-col">{terminal(active)}</div>;
  }

  const empty = active === null && layout === null;
  const focusNode = focusModePaneId === null || layout === null ? null : findPane(layout.root, focusModePaneId);
  return <div className="flex h-full min-h-0 flex-col"><div className="flex flex-wrap items-center gap-2 border-b border-line bg-toolbar px-3 py-2"><Button disabled={active?.kind !== "ssh" || focusModePaneId !== null} onClick={() => void split("horizontal")}>{t("workspace.splitRight")}</Button><Button disabled={active?.kind !== "ssh" || focusModePaneId !== null} onClick={() => void split("vertical")}>{t("workspace.splitDown")}</Button><label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={broadcast} onChange={(event) => setBroadcast(event.target.checked)} />{t("workspace.broadcast")}</label><Button disabled={commandTargets.length === 0} onClick={() => setCommandCenter((current) => !current)}>{t("workspace.commandCenter")}</Button>{focusModePaneId === null ? null : <Button onClick={() => setFocusModePaneId(null)}>{t("workspace.exitFocusMode")}</Button>}<select aria-label={t("workspace.saved")} value={selectedWorkspace} onChange={(event) => setSelectedWorkspace(event.target.value)} className="ml-auto rounded border border-control-line bg-control px-2 py-1 text-xs"><option value="">{t("workspace.new")}</option>{saved.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select><Button disabled={layout === null && active?.kind !== "ssh"} onClick={() => void saveWorkspace()}>{t("workspace.save")}</Button><Button disabled={selectedWorkspace === ""} onClick={() => void restoreWorkspace(selectedWorkspace)}>{t("workspace.reopen")}</Button><button disabled={selectedWorkspace === ""} className="text-xs text-danger disabled:opacity-40" onClick={() => void workspaceApi.remove(selectedWorkspace).then(async () => { setSelectedWorkspace(""); setSaved(await workspaceApi.list()); })}>{t("workspace.delete")}</button></div>{commandCenter && commandTargets.length > 0 ? <WorkspaceCommandCenter paneTargets={commandTargets} onClose={() => setCommandCenter(false)} /> : null}{problem === "" ? null : <p role="alert" className="bg-notice px-3 py-1 text-xs text-notice-ink">{problem}</p>}<div className="flex min-h-0 flex-1 flex-col">{empty ? <div className="flex h-full items-center justify-center p-6"><section className="sshc-card w-full max-w-md rounded-2xl bg-card p-8 text-center" role="status"><BrandMark className="mx-auto size-12 drop-shadow-sm" /><h2 className="mt-4 text-xl font-semibold tracking-tight text-ink">{t("terminal.emptyHeading")}</h2><p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-ink-muted">{t("terminal.emptyHint")}</p><div aria-hidden="true" className="mx-auto mt-5 max-w-xs rounded-lg bg-term-bg px-4 py-3 text-left font-mono text-xs text-ink shadow-inner"><span className="text-live">$</span> sshc host<span className="ml-1 inline-block h-3 w-1.5 translate-y-0.5 bg-ink" /></div></section></div> : layout === null ? (active === null ? null : terminal(active)) : focusNode === null ? renderNode(layout.root) : renderNode({ pane: focusNode })}</div></div>;
}
