import type { ShortcutAction } from "../keyconfig/bindings";
import type { RunTargetLink } from "../terminal/useRunTargetLink";
import type { RunTargetsState } from "../terminal/useRunTargets";

// runTargetNumberActions は、リモートセッションの一覧の1〜9番目へ切り替えるショートカットの操作。添字が一覧の並び
// （runTargetChoices）に当たる。
export const runTargetNumberActions = [
  "runTarget1", "runTarget2", "runTarget3", "runTarget4", "runTarget5", "runTarget6", "runTarget7", "runTarget8", "runTarget9",
] as const satisfies readonly ShortcutAction[];

// runTargetChoices は、リモートセッションの一覧に並ぶ順（先頭がこのマシンの null）。ショートカットの番号もこの順。
export function runTargetChoices(engines: readonly string[]): readonly (string | null)[] {
  return [null, ...engines];
}

// chooseRunTarget は、リモートセッションの一覧の行を押したときと同じことをする。サイドバーの一覧、ショートカット、
// コマンドパレットのどれから選んでも同じに動くよう、3つともここを呼ぶ。
export function chooseRunTarget({ targets, link, showTerminal }: {
  targets: Pick<RunTargetsState, "selected" | "select" | "vaultUnlock">;
  link: Pick<RunTargetLink, "phase" | "refusal">;
  // showTerminal はターミナルの領域を見せる。接続の経過と、断られた理由と直し方はそこに出る。
  showTerminal: () => void;
}, alias: string | null): void {
  // 選んでいるのに使えない sshc エンジンをもう一度選ぶと、直し方へ行く。理由は一覧の行に出ていて、直す操作のパネルは
  // ターミナルの領域にあり、ほかの画面からは見えない。ロックされた Vault は、その場のダイアログで解除する。
  if (alias !== null && alias === targets.selected && link.phase === "refused") {
    if (link.refusal?.problem?.code === "remote_engine_vault_locked") targets.vaultUnlock.ask(alias);
    else showTerminal();
    return;
  }
  targets.select(alias);
  // SSH の接続先の sshc エンジンには接続が要り、その経過はターミナルの領域に出る。セッションが無いと分かれば、
  // Home がターミナルに代わる（useHomeWithoutSessions）。
  if (alias !== null) showTerminal();
}
