import { useEffect, useMemo, useState, type ReactNode } from "react";
import type { TerminalSession } from "../../api/integrations";
import { failureCode } from "../../api/client";
import { useTranslate } from "../../i18n/context";
import { Button } from "../../ui/surface";
import { BrandMark } from "../../ui/BrandMark";
import { paneIDs, reduceLayout, restoreLayout, storeLayout, type LayoutAction, type LayoutState, type RuntimeNode, type RuntimePane, type SplitDirection, type StoredNode } from "./layout";
import { workspaceApi, type SavedWorkspace } from "./api";

export type BroadcastInput = { sequence: number; source: string; data: string };

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
  sessions, activeSessionId, onActive, onOpenAlias, renderTerminal,
}: {
  sessions: TerminalSession[];
  activeSessionId: string | null;
  onActive: (id: string) => void;
  onOpenAlias: (alias: string) => Promise<TerminalSession | null>;
  renderTerminal: (session: TerminalSession, onInput: (data: string) => void, injected: BroadcastInput | null) => ReactNode;
}) {
  const t = useTranslate();
  const [layout, setLayout] = useState<LayoutState | null>(null);
  const [saved, setSaved] = useState<SavedWorkspace[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [broadcast, setBroadcast] = useState(false);
  const [input, setInput] = useState<BroadcastInput | null>(null);
  const [problem, setProblem] = useState("");
  const active = sessions.find((session) => session.id === activeSessionId) ?? null;

  const sessionByID = useMemo(() => new Map(sessions.map((session) => [session.id, session])), [sessions]);
  useEffect(() => { void workspaceApi.list().then(setSaved).catch(() => undefined); }, []);

  function update(action: LayoutAction) { setLayout((current) => current === null ? current : reduceLayout(current, action)); }

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

  async function restore() {
    if (selectedWorkspace === "") return;
    try {
      const stored = await workspaceApi.restore(selectedWorkspace);
      setLayout(restoreLayout(stored.layout, stored.focusedPaneId));
      visit(stored.layout, (id, alias) => {
        update({ type: "connection-starting", paneId: id });
        void onOpenAlias(alias).then((session) => {
          if (session === null) update({ type: "connection-failed", paneId: id, problem: "open_failed" });
          else { update({ type: "connection-started", paneId: id, sessionId: session.id }); if (id === stored.focusedPaneId) onActive(session.id); }
        });
      });
      setProblem("");
    } catch (error) { setProblem(failureCode(error) || "workspace_failed"); }
  }

  function terminal(session: TerminalSession) {
    return renderTerminal(session, (data) => {
      if (broadcast) setInput((current) => ({ sequence: (current?.sequence ?? 0) + 1, source: session.id, data }));
    }, input?.source === session.id ? null : input);
  }

  function renderNode(node: RuntimeNode, path: ("first" | "second")[] = []): ReactNode {
    if (node.pane !== undefined) {
      const session = node.pane.sessionId === undefined ? undefined : sessionByID.get(node.pane.sessionId);
      return <div key={node.pane.id} className={`relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden border ${layout?.focusedPaneId === node.pane.id ? "border-accent" : "border-line"}`} onPointerDown={() => { update({ type: "focus", paneId: node.pane.id }); if (session !== undefined) onActive(session.id); }}>{session === undefined ? <div className="grid h-full min-h-40 place-items-center text-sm text-ink-muted">{node.pane.state === "failed" ? node.pane.problem : t("workspace.reconnecting")}</div> : terminal(session)}{layout !== null && paneIDs(layout.root).length > 1 ? <button type="button" aria-label={t("workspace.closePane")} className="absolute right-2 top-2 z-10 rounded bg-toolbar px-1.5 text-xs" onClick={(event) => { event.stopPropagation(); update({ type: "close", paneId: node.pane.id }); }}>×</button> : null}</div>;
    }
    const row = node.split.direction === "horizontal";
    return <div className={`flex h-full min-h-0 min-w-0 flex-1 ${row ? "flex-row" : "flex-col"}`}><div style={{ flexBasis: `${node.split.ratio}%` }} className="flex min-h-0 min-w-0">{renderNode(node.split.first, [...path, "first"])}</div><div role="separator" aria-orientation={row ? "vertical" : "horizontal"} className={row ? "w-1 cursor-col-resize bg-line" : "h-1 cursor-row-resize bg-line"} /><div style={{ flexBasis: `${100 - node.split.ratio}%` }} className="flex min-h-0 min-w-0">{renderNode(node.split.second, [...path, "second"])}</div></div>;
  }

  // A local shell does not participate in an SSH workspace. Keep its terminal
  // surface identical to the single-console path so fitting and selection are
  // unaffected by controls that cannot apply to it.
  if (layout === null && active?.kind === "shell") {
    return <div className="flex h-full min-h-0 flex-col">{terminal(active)}</div>;
  }

  const empty = active === null && layout === null;
  return <div className="flex h-full min-h-0 flex-col"><div className="flex flex-wrap items-center gap-2 border-b border-line bg-toolbar px-3 py-2"><Button disabled={active?.kind !== "ssh"} onClick={() => void split("horizontal")}>{t("workspace.splitRight")}</Button><Button disabled={active?.kind !== "ssh"} onClick={() => void split("vertical")}>{t("workspace.splitDown")}</Button><label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={broadcast} onChange={(event) => setBroadcast(event.target.checked)} />{t("workspace.broadcast")}</label><select aria-label={t("workspace.saved")} value={selectedWorkspace} onChange={(event) => setSelectedWorkspace(event.target.value)} className="ml-auto rounded border border-control-line bg-control px-2 py-1 text-xs"><option value="">{t("workspace.new")}</option>{saved.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select><Button disabled={layout === null && active?.kind !== "ssh"} onClick={() => void saveWorkspace()}>{t("workspace.save")}</Button><Button disabled={selectedWorkspace === ""} onClick={() => void restore()}>{t("workspace.reopen")}</Button><button disabled={selectedWorkspace === ""} className="text-xs text-danger disabled:opacity-40" onClick={() => void workspaceApi.remove(selectedWorkspace).then(async () => { setSelectedWorkspace(""); setSaved(await workspaceApi.list()); })}>{t("workspace.delete")}</button></div>{problem === "" ? null : <p role="alert" className="bg-notice px-3 py-1 text-xs text-notice-ink">{problem}</p>}<div className="flex min-h-0 flex-1 flex-col">{empty ? <div className="flex h-full items-center justify-center p-6"><section className="sshc-card w-full max-w-md rounded-2xl bg-card p-8 text-center" role="status"><BrandMark className="mx-auto size-12 drop-shadow-sm" /><h2 className="mt-4 text-xl font-semibold tracking-tight text-ink">{t("terminal.emptyHeading")}</h2><p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-ink-muted">{t("terminal.emptyHint")}</p><div aria-hidden="true" className="mx-auto mt-5 max-w-xs rounded-lg bg-term-bg px-4 py-3 text-left font-mono text-xs text-ink shadow-inner"><span className="text-live">$</span> ssh host<span className="ml-1 inline-block h-3 w-1.5 translate-y-0.5 bg-ink" /></div></section></div> : layout === null ? (active === null ? null : terminal(active)) : renderNode(layout.root)}</div></div>;
}
