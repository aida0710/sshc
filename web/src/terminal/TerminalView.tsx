import { useEffect, useRef, useState, type CSSProperties } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalSession } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { fontStack } from "./fonts";
import { defaultTint } from "./appearance";
import { useBackgroundImage } from "./backgroundImage";
import { clipboard } from "../ui/clipboard";
import { attachImeKeys } from "./imeKeys";
import { attachSelectionOverlay, selectionHeldIn } from "./selectionOverlay";
import { prefersNativeSelection } from "./nativeSelection";
import { cellHeight } from "./metrics";
import { newTouchScroll } from "./touchScroll";
import { KeyBar, applyModifiers, encodeKey, type Modifiers } from "./KeyBar";
import { openStream, type TerminalStream } from "./stream";
import { attachTerminalClipboard, type TerminalClipboardSettings } from "./clipboard";

type TerminalViewProps = {
  session: TerminalSession;
  api?: Pick<IntegrationsApi, "terminalStreamTicket">;
  onExit?: () => void;
  copyOnSelect?: boolean;
  fontSize?: number;
  rightClickPaste?: boolean;
  palette?: string;
  background?: string;
  tint?: number;
  font?: string;
  onInput?: (data: string) => void;
  injectedInput?: { sequence: number; data: string } | null;
};

type Link =
  | { phase: "live" }
  | { phase: "connecting"; attempt: number }
  | { phase: "waiting"; attempt: number; seconds: number }
  | { phase: "stopped"; gone: boolean };

const backoff = [1, 2, 4, 8, 15];

const settled = 10_000;

export function TerminalView({
  session,
  api = integrationsApi,
  onExit,
  copyOnSelect = true,
  fontSize,
  rightClickPaste = true,
  palette,
  font,
  background,
  tint,
  onInput,
  injectedInput,
}: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const host = useRef<HTMLDivElement>(null);
  const backgroundURL = useBackgroundImage(background ?? "");
  const hasBackground = backgroundURL !== "";
  const refit = useRef<(() => void) | null>(null);
  const terminal = useRef<Terminal | null>(null);
  const clipboardSettings = useRef<TerminalClipboardSettings>({ copyOnSelect, rightClickPaste });
  clipboardSettings.current = { copyOnSelect, rightClickPaste };
  const [problem, setProblem] = useState("");
  const [link, setLink] = useState<Link>({ phase: "connecting", attempt: 1 });
  const control = useRef<{ now: () => void; stop: () => void }>({ now: () => {}, stop: () => {} });

  const [modifiers, setModifiers] = useState<Modifiers>({ ctrl: false, alt: false });
  const armed = useRef<Modifiers>(modifiers);
  armed.current = modifiers;
  const send = useRef<(label: string) => void>(() => {});
  const inject = useRef<(data: string) => void>(() => {});
  const inputCallback = useRef(onInput);
  inputCallback.current = onInput;

  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    const coarse = prefersNativeSelection((query) => window.matchMedia(query));

    const view = new Terminal({
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.exited === undefined,
      fontFamily: fontStack(font ?? ""),
      fontSize: fontSize ?? (window.matchMedia("(max-width: 767px)").matches ? 15 : 13),
      theme: terminalTheme(container, hasBackground),
      scrollback: 5000,
    });
    const fit = new FitAddon();
    view.loadAddon(fit);
    view.open(container);
    terminal.current = view;

    let stream: TerminalStream | null = null;
    let live = true;
    let stopped = false;
    let attempts = 0;
    let linkedAt = 0;
    let timer: ReturnType<typeof setInterval> | undefined;
    const decoder = new TextDecoder();

    let sent = "";
    const syncSize = () => {
      const size = `${view.cols}x${view.rows}`;
      if (stream === null || view.cols === 0 || view.rows === 0 || size === sent) return;
      sent = size;
      stream.resize(view.cols, view.rows);
    };

    const measure = () => {
      if (container.clientWidth === 0 || container.clientHeight === 0) return;
      if (selectionHeldIn(container)) return;
      try {
        fit.fit();
      } catch {
        return;
      }
      syncSize();
    };
    measure();
    refit.current = measure;

    const scroll = newTouchScroll(view, () => cellHeight(view, container));
    const single = (event: TouchEvent): Touch | null =>
      event.touches.length === 1 ? (event.touches[0] ?? null) : null;
    const touchStart = (event: TouchEvent) => {
      const finger = single(event);
      if (finger !== null && !selectionHeldIn(container)) scroll.start(finger.clientY);
    };
    const touchMove = (event: TouchEvent) => {
      const finger = single(event);
      if (finger !== null && !selectionHeldIn(container)) scroll.move(finger.clientY);
    };
    container.addEventListener("touchstart", touchStart, { passive: true });
    container.addEventListener("touchmove", touchMove, { passive: true });


    const releaseImeKeys = coarse
      ? attachImeKeys({ container, textarea: view.textarea ?? container })
      : () => {};

    const detachOverlay = coarse ? attachSelectionOverlay(container, view) : () => {};

    const typed = (data: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = applyModifiers(data, ctrl, alt);
      stream?.send(encoded);
      inputCallback.current?.(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    send.current = (label: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = encodeKey(label, ctrl, alt);
      stream?.send(encoded);
      inputCallback.current?.(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    inject.current = (data: string) => stream?.send(data);
    view.onData(typed);

    const attach = () => {
      clearInterval(timer);
      stopped = false;
      attempts += 1;
      setLink({ phase: "connecting", attempt: attempts });
      void api
        .terminalStreamTicket(session.id)
        .then((issued) => {
          if (!live || stopped) return;
          sent = "";
          linkedAt = Date.now();
          stream = openStream(issued.streamTicket, {
            onOutput: (chunk) => view.write(decoder.decode(chunk, { stream: true })),
            onExit: () => {
              view.options.cursorBlink = false;
              stopped = true;
              setLink({ phase: "live" });
              onExit?.();
            },
            onClose: () => {
              stream = null;
              retry();
            },
          });
          setLink({ phase: "live" });
          if (session.exited === undefined) view.focus();
          syncSize();
        })
        .catch((error: unknown) => {
          if (!live || stopped) return;
          if (failureCode(error) === "terminal_session_not_found") {
            setLink({ phase: "stopped", gone: true });
            return;
          }
          retry();
        });
    };

    const retry = () => {
      if (!live || stopped) return;
      if (linkedAt !== 0 && Date.now() - linkedAt > settled) attempts = 1;
      let left = backoff[Math.min(attempts - 1, backoff.length - 1)] ?? 1;
      const next = attempts + 1;
      setLink({ phase: "waiting", attempt: next, seconds: left });
      timer = setInterval(() => {
        left -= 1;
        if (left > 0) {
          setLink({ phase: "waiting", attempt: next, seconds: left });
          return;
        }
        attach();
      }, 1000);
    };

    control.current = {
      now: attach,
      stop: () => {
        stopped = true;
        clearInterval(timer);
        setLink({ phase: "stopped", gone: false });
      },
    };
    attach();

    const detachClipboard = attachTerminalClipboard({
      container,
      terminal: view,
      clipboard,
      coarsePointer: () => coarse,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
    });

    const observer = new ResizeObserver(measure);
    observer.observe(container);

    return () => {
      live = false;
      clearInterval(timer);
      observer.disconnect();
      container.removeEventListener("touchstart", touchStart);
      container.removeEventListener("touchmove", touchMove);
      releaseImeKeys();
      detachOverlay();
      detachClipboard();
      stream?.close();
      view.dispose();
      terminal.current = null;
      inject.current = () => {};
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id, api]);

  useEffect(() => {
    if (injectedInput !== null && injectedInput !== undefined) inject.current(injectedInput.data);
  }, [injectedInput]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.fontFamily = fontStack(font ?? "");
    refit.current?.();
  }, [font]);

  return (
    <section aria-label={t("terminal.screenLabel", { title: session.title })} className="relative flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
        <span
          aria-hidden="true"
          className={`size-2 shrink-0 rounded-full ${
            session.exited !== undefined ? "bg-ink-faint" : link.phase === "live" ? "bg-live" : "bg-notice-ink"
          }`}
        />
        <p className="min-w-0 truncate font-mono text-xs font-semibold text-ink">{session.title}</p>
        {session.alias === undefined || session.alias === session.title ? null : (
          <span className="truncate rounded-md bg-surface px-2 py-0.5 font-mono text-[11px] text-ink-muted">
            {session.alias}
          </span>
        )}
      </div>
      {problem === "" ? null : (
        <p role="status" className="shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          {problem}
        </p>
      )}

      {link.phase === "live" ? null : (
        <div
          role="status"
          className="flex shrink-0 items-center gap-2 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink"
        >
          <p className="min-w-0 grow">
            {link.phase === "connecting"
              ? link.attempt === 1
                ? t("terminal.linkConnecting")
                : t("terminal.linkRetrying", { attempt: String(link.attempt) })
              : link.phase === "waiting"
                ? t("terminal.linkWaiting", { seconds: String(link.seconds), attempt: String(link.attempt) })
                : link.gone
                  ? t("terminal.linkGone")
                  : t("terminal.linkStopped")}
          </p>
          {link.phase === "stopped" && link.gone ? null : (
            <button
              type="button"
              disabled={link.phase === "connecting"}
              onClick={() => control.current.now()}
              className="shrink-0 rounded border border-notice-line px-2 py-0.5 text-notice-ink disabled:opacity-50"
            >
              {t("terminal.linkNow")}
            </button>
          )}
          {link.phase === "stopped" ? null : (
            <button
              type="button"
              onClick={() => control.current.stop()}
              className="shrink-0 rounded border border-notice-line px-2 py-0.5 text-notice-ink"
            >
              {t("terminal.linkStop")}
            </button>
          )}
        </div>
      )}
      {session.exited === undefined ? null : (
        <p role="status" className="shrink-0 border-b border-line bg-card px-3 py-1.5 text-xs text-ink-muted">
          {session.exited.signal === ""
            ? t("terminal.exitedWithCode", { code: String(session.exited.code) })
            : t("terminal.exitedWithSignal", { signal: session.exited.signal })}
        </p>
      )}


      <div
        ref={host}
        {...(palette === undefined || palette === "" ? {} : { "data-term-palette": palette })}
        {...(font === undefined || font === "" ? {} : { "data-term-font": font })}
        {...(hasBackground ? { "data-term-background": background ?? "" } : {})}
        style={
          hasBackground
            ? {
                "--ui-term-image": `url("${backgroundURL}")`,
                "--ui-term-tint": String(tint ?? defaultTint),
              } as CSSProperties
            : undefined
        }
        className="relative min-h-0 flex-1 bg-term-bg p-2"
      />
      <KeyBar
        modifiers={modifiers}
        onToggle={(name) => setModifiers((current) => ({ ...current, [name]: !current[name] }))}
        onKey={(label) => send.current(label)}
      />
    </section>
  );
}
