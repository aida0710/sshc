import { useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { runTargetRefusalState, runTargetRefusalText } from "../terminal/runTargetProblem";
import type { RunTargetLink } from "../terminal/useRunTargetLink";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { Icon } from "../ui/icons";
import { menuItemClass } from "../ui/menuStyle";
import { Notice } from "../ui/surface";
import { Spinner } from "../ui/Spinner";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { EngineInfoDialog } from "./EngineInfoDialog";
import { RemoteVaultUnlockDialog } from "./RemoteVaultUnlockDialog";
import { HostPickerDialog } from "./HostPickerDialog";

// RunTargetSwitcher sits at the bottom of the sidebar and chooses whose
// terminals the sidebar and the terminal show: this machine, or an engine on
// an SSH host that this machine's engine forwards to. Settings and SFTP stay
// this machine's whichever is chosen. It stays in view on every page, so it also says how the connection to the chosen
// one is going.
export function RunTargetSwitcher({
  targets,
  link,
  aliases,
  hosts,
  onShowTerminal,
}: {
  targets: RunTargetsState;
  link: RunTargetLink;
  aliases: string[];
  hosts: HostEntry[];
  // onShowTerminal shows the terminal area, where a refusal of the chosen engine is shown with what fixes it.
  onShowTerminal: () => void;
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  // actionsFor is the engine whose actions are unfolded under its row. They
  // unfold in place: a menu of its own would sit outside this popover and
  // close it.
  const [actionsFor, setActionsFor] = useState<string | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  useDismissibleLayer({ open, containerRefs: [rootRef], onDismiss: () => setOpen(false), returnFocusRef: triggerRef });

  const current = targets.selected ?? t("runTarget.thisMachine");
  const chosenState = linkStateWords();
  const choose = (alias: string | null) => {
    setOpen(false);
    // The chosen engine cannot be used: pressing it again goes to what fixes it. Its row already says why, and the
    // panel that offers the actions is in the terminal area, out of sight from any other page. A locked Vault is
    // unlocked here, in its own dialog.
    if (alias !== null && alias === targets.selected && link.phase === "refused") {
      if (link.refusal?.problem?.code === "remote_engine_vault_locked") targets.vaultUnlock.ask(alias);
      else onShowTerminal();
      return;
    }
    targets.select(alias);
  };
  // linkStateWords says what is happening to the chosen one; null when nothing needs saying.
  function linkStateWords(): string | null {
    switch (link.phase) {
      case "ready": return null;
      case "loading": return t("runTarget.stateLoading");
      case "connecting":
      case "awaitingEngine": return t("runTarget.stateConnecting");
      case "refused": return runTargetRefusalState(t, link.refusal);
    }
  }
  const row = (alias: string | null) => {
    const name = alias ?? t("runTarget.thisMachine");
    const chosen = targets.selected === alias;
    const connected = alias !== null && link.connectedEngines.has(alias);
    const settledNote = alias === null ? t("runTarget.thisMachineNote") : connected ? t("runTarget.stateConnected") : t("runTarget.stateNotConnected");
    const note = (chosen ? chosenState : null) ?? settledNote;
    // The note already says it could not connect; this adds why, when that is known.
    const refusalReason = chosen && link.refusal !== null ? runTargetRefusalText(t, link.refusal, name).reason : "";
    return (
      <li key={alias ?? ""} className={`flex flex-wrap items-center gap-1 rounded-md ${chosen ? "bg-select-fill" : "hover:bg-hover"}`}>
        <button
          type="button"
          aria-current={chosen ? "true" : undefined}
          onClick={() => choose(alias)}
          className="flex min-h-10 min-w-0 grow basis-0 items-center gap-2 px-2 text-left text-sm md:min-h-7"
        >
          <LinkMark mark={chosen ? chosenMark(link) : alias === null || connected ? "live" : "idle"} />
          <span title={name} className={`min-w-0 grow truncate font-mono ${chosen ? "font-semibold text-accent" : "text-ink"}`}>{name}</span>
          {alias !== null && targets.unread.has(alias) ? <UnreadMark label={t("runTarget.unread")} /> : null}
          <span className={`shrink-0 text-[11px] ${chosen && link.phase === "refused" ? "text-danger" : "text-ink-faint"}`}>{note}</span>
        </button>
        {alias === null ? null : (
          <button
            type="button"
            aria-label={t("runTarget.actions", { alias })}
            aria-expanded={actionsFor === alias}
            onClick={() => setActionsFor((currentAlias) => currentAlias === alias ? null : alias)}
            className="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-6"
          >
            <Icon name="moreHorizontal" className="size-3.5" />
          </button>
        )}
        {refusalReason === "" ? null : (
          <p className="basis-full break-words px-2 pb-1 text-[11px] leading-4 text-danger">{refusalReason}</p>
        )}
        {alias === null || actionsFor !== alias ? null : (
          <div className="basis-full pb-1 pl-5">
            <button type="button" onClick={() => { setActionsFor(null); setOpen(false); targets.engineInfo.ask(alias); }} className={menuItemClass()}>
              {t("engineInfo.open")}
            </button>
            <button type="button" onClick={() => void targets.openInNewTab(alias)} className={menuItemClass()}>
              {t("runTarget.openInNewTab")}
            </button>
            <button type="button" onClick={() => { setActionsFor(null); void targets.remove(alias); }} className={menuItemClass("danger")}>
              {t("runTarget.remove")}
            </button>
          </div>
        )}
      </li>
    );
  };

  return (
    <div ref={rootRef} className="relative pt-2">
      {open ? (
        <div className="absolute bottom-full left-1 right-1 z-20 mb-1 rounded-lg border border-line bg-card p-1.5 shadow-float">
          <p className="px-2 pb-1 pt-0.5 text-[11px] font-semibold text-ink-faint">{t("runTarget.heading")}</p>
          <ul aria-label={t("runTarget.heading")} className="flex flex-col gap-0.5">
            {row(null)}
            {targets.engines.map((alias) => row(alias))}
          </ul>
          <div className="my-1 border-t border-line" />
          <button
            type="button"
            disabled={targets.busy}
            onClick={() => setPicking(true)}
            className="flex min-h-10 w-full items-center gap-2 rounded-md px-2 text-left text-sm text-ink-muted hover:bg-hover md:min-h-7"
          >
            <Icon name="plus" className="size-3.5" aria-hidden="true" />
            {targets.busy ? t("runTarget.adding") : t("runTarget.add")}
          </button>
        </div>
      ) : null}
      {targets.problem === "" ? null : <Notice tone="danger" compact>{targets.problem}</Notice>}
      <button
        ref={triggerRef}
        type="button"
        aria-expanded={open}
        aria-label={chosenState === null ? t("runTarget.choose", { name: current }) : t("runTarget.chooseWhile", { name: current, state: chosenState })}
        onClick={() => setOpen((value) => !value)}
        className={`flex h-10 w-full items-center gap-2 rounded-lg bg-well px-2.5 ${open ? "ring-1 ring-accent" : ""}`}
      >
        <LinkMark mark={chosenMark(link)} />
        <span className="flex min-w-0 grow flex-col text-left">
          <span className={`truncate text-[11px] leading-[15px] ${link.phase === "refused" ? "text-danger" : "text-ink-muted"}`}>{chosenState ?? t("runTarget.label")}</span>
          <span title={current} className="truncate font-mono text-[12.5px] font-semibold leading-4 text-ink">{current}</span>
        </span>
        {targets.unread.size > 0 ? <UnreadMark label={t("runTarget.unreadElsewhere")} /> : null}
        <Icon name="chevronRight" className={`size-3.5 shrink-0 text-ink-faint transition-transform ${open ? "-rotate-90" : "rotate-90"}`} />
      </button>
      <HostPickerDialog
        open={picking}
        heading={t("runTarget.addHeading")}
        aliases={aliases}
        hosts={hosts}
        returnFocusRef={triggerRef}
        onChoose={(alias) => {
          setPicking(false);
          void targets.add(alias).then((added) => { if (added) setOpen(false); });
        }}
        onClose={() => setPicking(false)}
      />
      {targets.engineInfo.openFor === null ? null : (
        <EngineInfoDialog key={targets.engineInfo.openFor} alias={targets.engineInfo.openFor} targets={targets} returnFocusRef={triggerRef} onClose={targets.engineInfo.close} />
      )}
      {/* After the information dialog so that, asked from there, it opens above it. */}
      {targets.vaultUnlock.unlockFor === null ? null : (
        <RemoteVaultUnlockDialog
          alias={targets.vaultUnlock.unlockFor}
          api={targets.api}
          onUnlocked={targets.vaultUnlock.unlocked}
          onOpenInNewTab={() => {
            const alias = targets.vaultUnlock.unlockFor;
            targets.vaultUnlock.close();
            if (alias !== null) void targets.openInNewTab(alias);
          }}
          onClose={targets.vaultUnlock.close}
        />
      )}
    </div>
  );
}

type LinkMarkKind = "live" | "idle" | "waiting" | "failed";

function chosenMark(link: RunTargetLink): LinkMarkKind {
  switch (link.phase) {
    case "ready": return "live";
    case "refused": return "failed";
    case "loading":
    case "connecting":
    case "awaitingEngine": return "waiting";
  }
}

// LinkMark shows the state of a connection beside its name, which says it in words too.
function LinkMark({ mark }: { mark: LinkMarkKind }) {
  if (mark === "waiting") return <Spinner className="size-2.5 text-ink-muted" />;
  const fill = { live: "bg-live", idle: "bg-ink-faint", failed: "bg-danger" }[mark];
  return <span aria-hidden="true" data-link-mark={mark} className={`size-1.5 shrink-0 rounded-full ${fill}`} />;
}

// UnreadMark is the dot for a run target whose terminals sent a notification nobody has seen yet.
function UnreadMark({ label }: { label: string }) {
  return (
    <span className="flex shrink-0 items-center">
      <span aria-hidden="true" className="size-2 rounded-full bg-accent" />
      <span className="sr-only">{label}</span>
    </span>
  );
}
