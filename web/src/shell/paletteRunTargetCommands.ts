import type { Translate } from "../i18n/context";
import type { Bindings } from "../keyconfig/bindings";
import type { PaletteCommand } from "./CommandPalette";
import { runTargetNumberActions } from "./runTargetChoice";

// paletteRunTargetCommands は、リモートセッションを名前で選ぶ項目。一覧と同じ並びで、番号のショートカットを添える。
// スマホはサイドバーの一番下まで行かなくても、検索から切り替えられる。このマシンしか無い間は出さない。
export function paletteRunTargetCommands({ t, bindings, runTargets, selectedRunTarget, chooseRunTarget }: {
  t: Translate;
  bindings: Bindings;
  runTargets: readonly (string | null)[];
  selectedRunTarget: string | null;
  chooseRunTarget: (alias: string | null) => void;
}): PaletteCommand[] {
  if (runTargets.length < 2) return [];
  return runTargets.map((alias, index) => {
    const name = alias ?? t("runTarget.thisMachine");
    const numberAction = runTargetNumberActions[index];
    return {
      id: `run-target:${alias ?? ""}`,
      label: t("palette.runTarget", { name }),
      detail: alias === selectedRunTarget ? t("palette.inUse") : "",
      search: `remote session engine switch リモートセッション sshcエンジン 切り替え ${name}`,
      keys: numberAction === undefined ? [] : bindings[numberAction],
      run: () => chooseRunTarget(alias),
    };
  });
}
