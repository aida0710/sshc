import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ApiError } from "../api/client";
import { configApi } from "../api/config";
import {
  integrationsApi,
  type IntegrationsApi,
  type TerminalForward,
  type TerminalSession,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { control, hintText } from "../ui/form";
import { Button, Notice } from "../ui/surface";

type ForwardApi = Pick<IntegrationsApi, "startTerminalForward" | "stopTerminalForward">;
type SaveApi = Pick<typeof configApi, "overview" | "host" | "save">;

export function TerminalPortForwards({
  session,
  api = integrationsApi,
  saveApi = configApi,
  onChanged,
  onClose,
}: {
  session: TerminalSession;
  api?: ForwardApi;
  saveApi?: SaveApi;
  onChanged?: () => void | Promise<void>;
  onClose: () => void;
}) {
  const t = useTranslate();
  const [forwards, setForwards] = useState<TerminalForward[]>(session.forwards ?? []);
  const [kind, setKind] = useState<"local" | "dynamic">("local");
  const [listenPort, setListenPort] = useState("");
  const [destination, setDestination] = useState("");
  const [save, setSave] = useState(false);
  const [busy, setBusy] = useState(false);
  const [stopping, setStopping] = useState("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  useEffect(() => setForwards(session.forwards ?? []), [session.forwards]);
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy && stopping === "") onClose();
    };
    window.addEventListener("keydown", escape);
    return () => window.removeEventListener("keydown", escape);
  }, [busy, onClose, stopping]);

  const connected = session.state === "connected";
  const canSave = session.alias !== undefined && session.alias !== "";
  const listenError = listenPort === "" || validPort(listenPort) ? "" : t("conn.forwardInvalidPort");
  const destinationError = kind === "dynamic" || destination === "" || validDestination(destination)
    ? ""
    : t("conn.forwardInvalidDestination");
  const canStart = connected && validPort(listenPort) && (kind === "dynamic" || validDestination(destination));
  const title = useMemo(() => session.alias ?? session.title, [session.alias, session.title]);

  async function start() {
    if (!canStart) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const listed = await api.startTerminalForward(session.id, {
        kind,
        listenPort: Number(listenPort),
        ...(kind === "local" ? { destination } : {}),
      });
      const current = listed.sessions.find((candidate) => candidate.id === session.id);
      setForwards(current?.forwards ?? []);
      if (save) {
        try {
          await saveToConnection(kind, listenPort, destination, session, saveApi);
        } catch {
          setError(t("terminal.forwardSaveFailed"));
          setNotice(t("terminal.forwardStarted"));
          setListenPort("");
          setDestination("");
          await onChanged?.();
          return;
        }
      }
      setNotice(t(save ? "terminal.forwardStartedAndSaved" : "terminal.forwardStarted"));
      setListenPort("");
      setDestination("");
      await onChanged?.();
    } catch (caught) {
      setError(forwardError(t, caught));
    } finally {
      setBusy(false);
    }
  }

  async function stop(forward: TerminalForward) {
    if (forward.id === "") return;
    setStopping(forward.id);
    setError("");
    setNotice("");
    try {
      const listed = await api.stopTerminalForward(session.id, forward.id);
      const current = listed.sessions.find((candidate) => candidate.id === session.id);
      setForwards(current?.forwards ?? []);
      setNotice(t("terminal.forwardStopped"));
      await onChanged?.();
    } catch (caught) {
      setError(forwardError(t, caught));
    } finally {
      setStopping("");
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/80 p-3 backdrop-blur-sm">
      <section role="dialog" aria-modal="true" aria-labelledby="terminal-forward-heading" className="sshc-card flex max-h-full w-full max-w-2xl flex-col overflow-hidden rounded-lg bg-card shadow-xl">
        <header className="flex items-start gap-3 border-b border-line px-4 py-4 sm:px-5">
          <div className="min-w-0 grow">
            <h2 id="terminal-forward-heading" className="text-base font-semibold text-ink">{t("terminal.portForwarding")}</h2>
            <p className={`mt-1 ${hintText}`}>{t("terminal.forwardDescription", { title })}</p>
          </div>
          <Button disabled={busy || stopping !== ""} onClick={onClose}>{t("terminal.forwardClose")}</Button>
        </header>

        <div className="flex min-h-0 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
          {error === "" ? null : <Notice tone="danger">{error}</Notice>}
          {notice === "" ? null : <Notice>{notice}</Notice>}
          {session.state === "reconnecting" ? <Notice>{t("terminal.forwardPausedReconnect")}</Notice> : null}

          <section aria-labelledby="terminal-forward-active" className="flex flex-col gap-2">
            <h3 id="terminal-forward-active" className="text-sm font-medium text-ink">{t("terminal.forwardActive")}</h3>
            {forwards.length === 0 ? <p className={hintText}>{t("terminal.forwardNone")}</p> : (
              <div className="sshc-card overflow-hidden rounded-lg bg-surface-subtle">
                {forwards.map((forward) => (
                  <div key={forward.id || `${forward.kind}-${forward.listen}-${forward.to}`} className="flex flex-col gap-2 border-t border-hairline px-3 py-3 first:border-t-0 sm:flex-row sm:items-center">
                    <div className="min-w-0 grow">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className={`size-2 rounded-full ${forward.problem === "" ? "bg-live" : "bg-danger"}`} aria-hidden="true" />
                        <span className="text-sm font-medium text-ink">{forwardLabel(t, forward)}</span>
                        <span className="rounded bg-tree px-1.5 py-0.5 text-[10px] text-ink-muted">{t(forward.temporary ? "terminal.forwardTemporary" : "terminal.forwardSaved")}</span>
                      </div>
                      <p className="mt-1 break-all font-mono text-xs text-ink-muted">{forwardAddress(forward)}</p>
                      {forward.temporary || forward.kind === "agent" ? null : <p className={`mt-1 ${hintText}`}>{t("terminal.forwardSavedStopHint")}</p>}
                      {forward.problem === "" ? null : <p role="alert" className="mt-1 break-all text-xs text-danger">{forward.problem}</p>}
                      {forward.problem === "" ? null : <p className={`mt-1 ${hintText}`}>{t("terminal.forwardRetryHint")}</p>}
                    </div>
                    <div className="flex shrink-0 gap-2">
                      {forward.kind === "agent" ? null : <Button onClick={() => void copyForward(forward, setNotice, setError, t)}>{t("terminal.forwardCopy")}</Button>}
                      {forward.kind === "agent" || forward.id === "" || forward.problem !== "" ? null : (
                        <Button kind="danger" disabled={stopping !== ""} onClick={() => void stop(forward)}>
                          {stopping === forward.id ? t("terminal.forwardStopping") : t("terminal.forwardStop")}
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          <section aria-labelledby="terminal-forward-new" className="flex flex-col gap-3 border-t border-line pt-4">
            <h3 id="terminal-forward-new" className="text-sm font-medium text-ink">{t("terminal.forwardNew")}</h3>
            <Notice>{t("conn.forwardLoopbackOnly")}</Notice>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="flex flex-col gap-1 text-xs text-ink-muted">
                {t("conn.forwardType")}
                <select className={control} value={kind} disabled={!connected || busy} onChange={(event) => setKind(event.currentTarget.value as "local" | "dynamic")}>
                  <option value="local">{t("conn.forwardLocal")}</option>
                  <option value="dynamic">{t("conn.forwardDynamic")}</option>
                </select>
              </label>
              <label className="flex flex-col gap-1 text-xs text-ink-muted">
                {t("conn.forwardListenPort")}
                <input autoFocus inputMode="numeric" className={control} value={listenPort} disabled={!connected || busy} onChange={(event) => setListenPort(event.currentTarget.value)} />
                {listenError === "" ? null : <span className="text-danger">{listenError}</span>}
              </label>
              {kind === "local" ? (
                <label className="flex flex-col gap-1 text-xs text-ink-muted sm:col-span-2">
                  {t("conn.forwardDestination")}
                  <input aria-label={t("conn.forwardDestination")} className={control} placeholder="127.0.0.1:5432" value={destination} disabled={!connected || busy} onChange={(event) => setDestination(event.currentTarget.value)} />
                  {destinationError === "" ? null : <span className="text-danger">{destinationError}</span>}
                </label>
              ) : null}
            </div>
            <p className={hintText}>{t(kind === "local" ? "conn.forwardDestinationHint" : "conn.forwardDynamicHint")}</p>
            <label className={`flex items-start gap-2 text-sm ${canSave ? "text-ink" : "text-ink-muted"}`}>
              <input type="checkbox" checked={save} disabled={!canSave || busy} onChange={(event) => setSave(event.currentTarget.checked)} className="mt-0.5 size-4" />
              <span>
                {t("terminal.forwardSaveConnection")}
                <span className={`block ${hintText}`}>{t(canSave ? "terminal.forwardSaveHint" : "terminal.forwardSaveUnavailable")}</span>
              </span>
            </label>
            {!connected ? <Notice>{t("terminal.forwardNeedsConnection")}</Notice> : null}
            <div className="flex justify-end">
              <Button kind="primary" disabled={!canStart || busy} onClick={() => void start()}>
                {busy ? t("terminal.forwardStarting") : t("terminal.forwardStart")}
              </Button>
            </div>
          </section>
        </div>
      </section>
    </div>,
    document.body,
  );
}

async function saveToConnection(
  kind: "local" | "dynamic",
  listenPort: string,
  destination: string,
  session: TerminalSession,
  api: SaveApi,
) {
  if (session.alias === undefined || session.alias === "") throw new Error("connection_not_saved");
  const overview = await api.overview();
  const matches = overview.hosts.filter((host) => host.identity.alias === session.alias);
  if (matches.length !== 1) throw new Error("connection_not_unique");
  const identity = matches[0]?.identity;
  if (identity === undefined) throw new Error("connection_not_found");
  const detail = await api.host(identity.path, identity.alias);
  await api.save({
    kind: "host_fields",
    path: identity.path,
    alias: identity.alias,
    base: detail.file.contents,
    fields: [{
      action: "add",
      keyword: kind === "local" ? "LocalForward" : "DynamicForward",
      values: kind === "local" ? [listenPort, destination] : [listenPort],
    }],
  });
}

function forwardLabel(t: ReturnType<typeof useTranslate>, forward: TerminalForward) {
  if (forward.kind === "dynamic") return t("conn.forwardDynamic");
  if (forward.kind === "agent") return t("terminal.forwardAgentLabel");
  return t("conn.forwardLocal");
}

function forwardAddress(forward: TerminalForward): string {
  if (forward.kind === "dynamic") return `socks5://${forward.listen}`;
  if (forward.kind === "agent") return forward.listen;
  return `${forward.listen} → ${forward.to}`;
}

async function copyForward(
  forward: TerminalForward,
  setNotice: (value: string) => void,
  setError: (value: string) => void,
  t: ReturnType<typeof useTranslate>,
) {
  try {
    await clipboard.writeText(forward.kind === "dynamic" ? `socks5://${forward.listen}` : forward.listen);
    setError("");
    setNotice(t("terminal.forwardCopied"));
  } catch {
    setNotice("");
    setError(t("terminal.forwardCopyFailed"));
  }
}

function forwardError(t: ReturnType<typeof useTranslate>, caught: unknown): string {
  if (caught instanceof ApiError) {
    if (caught.code === "terminal_forward_bind_failed" && caught.problem?.detail) {
      return t("terminal.forwardBindFailed", { detail: caught.problem.detail });
    }
    if (caught.code === "terminal_forward_unavailable") return t("terminal.forwardUnavailable");
    if (caught.code === "invalid_terminal_forward" || caught.code === "invalid_request") return t("terminal.forwardInvalid");
  }
  return t("terminal.forwardFailed");
}

function validPort(value: string): boolean {
  const port = Number(value);
  return /^\d+$/.test(value) && Number.isInteger(port) && port > 0 && port <= 65535;
}

function validDestination(value: string): boolean {
  const bracketed = /^\[[^\]]+\]:(\d+)$/.exec(value);
  if (bracketed !== null) return validPort(bracketed[1] ?? "");
  const separator = value.lastIndexOf(":");
  if (separator <= 0 || value.slice(0, separator).includes(":")) return false;
  return validPort(value.slice(separator + 1));
}
