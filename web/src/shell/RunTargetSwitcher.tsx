import { useRef, useState } from "react";
import type { HostEntry, HostIdentity } from "../api/config";
import { useTranslate } from "../i18n/context";
import { runTargetRefusalCause, runTargetRefusalState } from "../terminal/runTargetProblem";
import type { RunTargetLink } from "../terminal/useRunTargetLink";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { Icon } from "../ui/icons";
import { Spinner } from "../ui/Spinner";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { EngineInfoDialog } from "./EngineInfoDialog";
import { RunTargetProblemNotice } from "../terminal/RunTargetProblemNotice";
import { RemoteVaultUnlockDialog } from "./RemoteVaultUnlockDialog";
import { HostPickerDialog } from "./HostPickerDialog";
import { chooseRunTarget, runTargetChoices } from "./runTargetChoice";
import { recentConnectionsApi } from "../api/recentConnections";
import { ActionMenu, hideOnHoverAtPcSize, revealOnHoverAtPcSize, type ActionMenuItem } from "../ui/ActionMenu";

// リモートセッションに足す sshc エンジンは、このマシンの接続先から選ぶ。最近使ったものも、このマシンのものを出す。
const loadThisMachineRecent = () => recentConnectionsApi.recentConnections();

// RunTargetSwitcher はサイドバーの一番下にあり、画面が使う sshc エンジンを選ぶ。このマシンか、このマシンの
// エンジンが取り次ぐ SSH の接続先のエンジンである。Snippets・設定・同期・更新は、どれを選んでもこのマシンのもの。
// どの画面でも見えているので、選んだものへの接続の経過もここに出す。
export function RunTargetSwitcher({
  targets,
  link,
  aliases,
  hosts,
  onShowTerminal,
  onOpenConnectionSettings,
}: {
  targets: RunTargetsState;
  link: RunTargetLink;
  aliases: string[];
  hosts: HostEntry[];
  // onShowTerminal shows the terminal area, where the connection to the chosen engine and a refusal of it are
  // shown, with what fixes it.
  onShowTerminal: () => void;
  // onOpenConnectionSettings は、その sshc エンジンの接続先（このマシンの Connections）の設定を開く。色はそこで変える。
  onOpenConnectionSettings: (identity: HostIdentity) => void;
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  // menuFor は、［…］のメニューを開いているエンジン。右クリックでも開くので、ここで持つ。
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  useDismissibleLayer({
    open,
    containerRefs: [rootRef],
    // 行の［…］のメニューは、この一覧の上に重なる層として開く。新しい層は下の層を閉じさせる（superseded）が、
    // 自分の行のメニューのときは一覧を開いたままにして、メニューを閉じたら一覧に戻る。
    onDismiss: (reason) => {
      if (reason === "superseded" && menuFor !== null) return;
      setOpen(false);
    },
    returnFocusRef: triggerRef,
  });

  const current = targets.selected ?? t("runTarget.thisMachine");
  const chosenState = linkStateWords();
  const choose = (alias: string | null) => {
    setOpen(false);
    chooseRunTarget({ targets, link, showTerminal: onShowTerminal }, alias);
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
  // engineActions は、一覧の行の［…］のメニューの項目。メニューは上に重なる層なので、選んでもこの一覧は閉じず、
  // 別の画面やダイアログへ移る項目だけが一覧を閉じる。
  function engineActions(alias: string, connection: HostEntry | undefined): ActionMenuItem[] {
    return [
      { label: t("engineInfo.open"), onSelect: () => { setOpen(false); targets.engineInfo.ask(alias); } },
      ...(connection === undefined ? [] : [{
        // リモートセッションを選んでいる間、Connections はリモートの接続先を出すので、localhost に戻って開く。
        label: t(targets.selected === null ? "runTarget.openConnectionSettings" : "runTarget.openConnectionSettingsLeaving"),
        onSelect: () => { setOpen(false); onOpenConnectionSettings(connection.identity); },
      }]),
      { label: t("runTarget.openInNewTab"), onSelect: () => void targets.openInNewTab(alias) },
      { label: t("runTarget.remove"), tone: "danger", onSelect: () => void targets.remove(alias) },
    ];
  }

  const row = (alias: string | null) => {
    const name = alias ?? t("runTarget.thisMachine");
    const chosen = targets.selected === alias;
    const connected = alias !== null && link.connectedEngines.has(alias);
    const settledNote = alias === null ? t("runTarget.thisMachineNote") : connected ? t("runTarget.stateConnected") : t("runTarget.stateNotConnected");
    const note = (chosen ? chosenState : null) ?? settledNote;
    // The note already says it could not connect; this adds why, when that is known.
    const refusalReason = chosen && link.refusal !== null ? runTargetRefusalCause(t, link.refusal, name) : "";
    // connection は、このマシンの SSH の設定にあるその接続先。消したものには、設定を開く項目を出さない。
    const connection = alias === null ? undefined : hosts.find((host) => host.identity.alias === alias);
    return (
      <li key={alias ?? ""}
        // 右クリックか長押しでも［…］のメニューを開く。PC の大きさの画面では［…］が乗せたときだけ出るため。
        onContextMenu={alias === null ? undefined : (event) => {
          if (menuFor === alias) return;
          event.preventDefault();
          setMenuFor(alias);
        }}
        className={`group/row relative flex flex-wrap items-center gap-1 rounded-md ${chosen ? "bg-select-fill" : "hover:bg-hover"}`}>
        <button
          type="button"
          aria-current={chosen ? "true" : undefined}
          onClick={() => choose(alias)}
          className="flex min-h-10 min-w-0 grow basis-0 items-center gap-2 px-2 text-left text-sm md:min-h-7"
        >
          <LinkMark mark={chosen ? chosenMark(link) : alias === null || connected ? "live" : "idle"} />
          <span title={name} className={`min-w-0 grow truncate font-mono ${chosen ? "font-semibold text-accent" : "text-ink"}`}>{name}</span>
          {alias !== null && targets.unread.has(alias) ? <UnreadMark label={t("runTarget.unread")} /> : null}
          {/* PC の大きさの画面で［…］を出している間は、状態の字を隠して場所を譲る。重ねると字が切れたように見える。 */}
          <span className={`shrink-0 text-2xs ${chosen && link.phase === "refused" ? "text-danger" : "text-ink-faint"} ${
            alias === null ? "" : menuFor === alias ? "desktop-layout:invisible" : hideOnHoverAtPcSize
          }`}>{note}</span>
        </button>
        {/* サイドバーのセッションの行と同じく、PC の大きさの画面では乗せた行（とキーボードで触れた行）にだけ［…］を出し、
            行の右の状態の字の上に重ねる。スマホの組み方では行の中に常に出す。 */}
        {alias === null ? null : (
          <div className={`flex shrink-0 items-center desktop-layout:absolute desktop-layout:right-1 desktop-layout:top-1 desktop-layout:rounded-md ${
            chosen ? "desktop-layout:bg-select-fill" : "desktop-layout:bg-hover"
          } ${menuFor === alias ? "" : revealOnHoverAtPcSize}`}>
            <ActionMenu
              label={t("runTarget.actions", { alias })}
              items={engineActions(alias, connection)}
              triggerSize="row"
              triggerTouchDense
              triggerClassName="focus:bg-select-fill"
              iconClassName="size-3.5"
              open={menuFor === alias}
              onOpenChange={(next) => setMenuFor(next ? alias : null)}
            />
          </div>
        )}
        {refusalReason === "" ? null : (
          <p className="basis-full break-words px-2 pb-1 text-2xs leading-4 text-danger">{refusalReason}</p>
        )}
      </li>
    );
  };

  return (
    <div ref={rootRef} className="relative pt-2">
      {open ? (
        <div className="absolute bottom-full left-1 right-1 z-20 mb-1 rounded-lg border border-line bg-card p-1.5 shadow-float">
          <p className="px-2 pb-1 pt-0.5 text-2xs font-semibold text-ink-faint">{t("runTarget.heading")}</p>
          <ul aria-label={t("runTarget.heading")} className="flex flex-col gap-0.5">
            {runTargetChoices(targets.engines).map((alias) => row(alias))}
          </ul>
          <div className="my-1 border-t border-line" />
          {/* busy-label-exempt: メニューの行の形のボタン。Button の busyLabel と同じく、足している間は「接続中…」に替える。 */}
          <button
            type="button"
            disabled={targets.busy}
            aria-busy={targets.busy || undefined}
            onClick={() => setPicking(true)}
            className="flex min-h-10 w-full items-center gap-2 rounded-md px-2 text-left text-sm text-ink-muted hover:bg-hover md:min-h-7"
          >
            {targets.busy ? <Spinner /> : <Icon name="plus" className="size-3.5" aria-hidden="true" />}
            {t(targets.busy ? "busy.connecting" : "runTarget.add")}
          </button>
        </div>
      ) : null}
      {targets.problem === null ? null : (
        <div className="mb-2">
          <RunTargetProblemNotice problem={targets.problem} onRetry={targets.retryProblem} onDismiss={targets.dismissProblem} />
        </div>
      )}
      <button
        ref={triggerRef}
        type="button"
        aria-expanded={open}
        aria-label={chosenState === null ? t("runTarget.choose", { name: current }) : t("runTarget.chooseWhile", { name: current, state: chosenState })}
        // 一覧は開くたびに［…］のメニューを閉じた形で出す。前に開いたまま一覧ごと閉じたメニューを、また開かない。
        onClick={() => { setMenuFor(null); setOpen((value) => !value); }}
        className={`flex h-10 w-full items-center gap-2 rounded-lg bg-well px-2.5 ${open ? "ring-1 ring-accent" : ""}`}
      >
        <LinkMark mark={chosenMark(link)} />
        <span className="flex min-w-0 grow flex-col text-left">
          <span className={`truncate text-2xs leading-[15px] ${link.phase === "refused" ? "text-danger" : "text-ink-muted"}`}>{chosenState ?? t("runTarget.label")}</span>
          <span title={current} className="truncate font-mono text-xs font-semibold leading-4 text-ink">{current}</span>
        </span>
        {targets.unread.size > 0 ? <UnreadMark label={t("runTarget.unreadElsewhere")} /> : null}
        <Icon name="chevronRight" className={`size-3.5 shrink-0 text-ink-faint transition-transform ${open ? "-rotate-90" : "rotate-90"}`} />
      </button>
      <HostPickerDialog
        open={picking}
        heading={t("runTarget.addHeading")}
        aliases={aliases}
        hosts={hosts}
        loadRecent={loadThisMachineRecent}
        returnFocusRef={triggerRef}
        onChoose={(alias) => {
          setPicking(false);
          // The added engine becomes the chosen one, so the terminal area is shown as for a chosen one.
          void targets.add(alias).then((added) => {
            if (!added) return;
            setOpen(false);
            onShowTerminal();
          });
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
