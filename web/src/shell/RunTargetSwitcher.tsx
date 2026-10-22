import { useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { Icon } from "../ui/icons";
import { Notice } from "../ui/surface";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { EngineInfoDialog } from "./EngineInfoDialog";
import { RemoteVaultUnlockDialog } from "./RemoteVaultUnlockDialog";
import { HostPickerDialog } from "./HostPickerDialog";

// RunTargetSwitcher sits at the bottom of the sidebar and chooses whose
// terminals the sidebar and the terminal show: this machine, or an engine on
// an SSH host that this machine's engine forwards to. Settings and SFTP stay
// this machine's whichever is chosen.
export function RunTargetSwitcher({
  targets,
  aliases,
  hosts,
}: {
  targets: RunTargetsState;
  aliases: string[];
  hosts: HostEntry[];
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  // actionsFor is the engine whose actions are unfolded under its row. They
  // unfold in place: a menu of its own would sit outside this popover and
  // close it.
  const [actionsFor, setActionsFor] = useState<string | null>(null);
  const [infoFor, setInfoFor] = useState<string | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  useDismissibleLayer({ open, containerRefs: [rootRef], onDismiss: () => setOpen(false), returnFocusRef: triggerRef });

  const current = targets.selected ?? t("runTarget.thisMachine");
  const choose = (alias: string | null) => {
    targets.select(alias);
    setOpen(false);
  };
  const row = (alias: string | null) => {
    const name = alias ?? t("runTarget.thisMachine");
    const chosen = targets.selected === alias;
    return (
      <li key={alias ?? ""} className={`flex flex-wrap items-center gap-1 rounded-md ${chosen ? "bg-select-fill" : "hover:bg-hover"}`}>
        <button
          type="button"
          aria-current={chosen ? "true" : undefined}
          onClick={() => choose(alias)}
          className="flex min-h-10 min-w-0 grow items-center gap-2 px-2 text-left text-sm md:min-h-7"
        >
          <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-live" />
          <span className={`min-w-0 grow truncate font-mono ${chosen ? "font-semibold text-accent" : "text-ink"}`}>{name}</span>
          {alias === null ? <span className="shrink-0 text-[11px] text-ink-faint">{t("runTarget.thisMachineNote")}</span> : null}
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
        {alias === null || actionsFor !== alias ? null : (
          <div className="basis-full pb-1 pl-5">
            <button type="button" onClick={() => { setActionsFor(null); setOpen(false); setInfoFor(alias); }} className="block min-h-10 w-full rounded px-2 text-left text-xs text-ink hover:bg-hover md:min-h-7">
              {t("engineInfo.open")}
            </button>
            <button type="button" onClick={() => void targets.openInNewTab(alias)} className="block min-h-10 w-full rounded px-2 text-left text-xs text-ink hover:bg-hover md:min-h-7">
              {t("runTarget.openInNewTab")}
            </button>
            <button type="button" onClick={() => { setActionsFor(null); void targets.remove(alias); }} className="block min-h-10 w-full rounded px-2 text-left text-xs text-danger hover:bg-hover md:min-h-7">
              {t("runTarget.remove")}
            </button>
          </div>
        )}
      </li>
    );
  };

  return (
    <div ref={rootRef} className="relative border-t border-line px-1 pb-1 pt-2">
      {open ? (
        <div className="absolute bottom-full left-1 right-1 z-20 mb-1 rounded-lg border border-line bg-card p-1.5 shadow-lg">
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
            className="flex min-h-10 w-full items-center gap-2 rounded-md px-2 text-left text-sm text-ink-muted hover:bg-hover disabled:text-ink-faint md:min-h-7"
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
        aria-label={t("runTarget.choose", { name: current })}
        onClick={() => setOpen((value) => !value)}
        className={`flex h-10 w-full items-center gap-2 rounded-md border bg-control px-2.5 text-sm md:h-8 ${open ? "border-accent" : "border-line"}`}
      >
        <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-live" />
        <span className="shrink-0 text-xs text-ink-faint">{t("runTarget.label")}</span>
        <span className="min-w-0 grow truncate text-left font-mono font-semibold text-accent">{current}</span>
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
      {infoFor === null ? null : (
        <EngineInfoDialog alias={infoFor} targets={targets} returnFocusRef={triggerRef} onClose={() => setInfoFor(null)} />
      )}
      {/* After the information dialog so that, asked from there, it opens above it. */}
      {targets.vaultUnlock.unlockFor === null ? null : (
        <RemoteVaultUnlockDialog
          alias={targets.vaultUnlock.unlockFor}
          api={targets.api}
          localVersion={targets.localVersion}
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
