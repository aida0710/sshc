import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Button } from "../ui/surface";
import type { MoveOutcome, MoveTarget } from "./organizer";

// ChosenKeysBar は、チェックした鍵をまとめてフォルダへ移すための帯である。
// target の "" は、どのグループにも入れないことを表す。
export function ChosenKeysBar({
  count,
  groups,
  target,
  onTargetChange,
  onMove,
  onClear,
}: {
  count: number;
  groups: string[];
  target: string;
  onTargetChange: (target: string) => void;
  onMove: (target: MoveTarget) => void;
  onClear: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-control-line bg-card p-3">
      <p className="grow text-sm text-ink">{t("keys.chosenCount", { count })}</p>
      <select
        aria-label={t("keys.moveTargetLabel")}
        className={control}
        value={target}
        onChange={(event) => onTargetChange(event.target.value)}
      >
        <option value="">{t("keys.folderUngrouped")}</option>
        {groups.map((group) => (
          <option key={group} value={group}>
            {group}
          </option>
        ))}
      </select>
      <Button
        kind="primary"
        onClick={() => onMove(target === "" ? { kind: "ungrouped" } : { kind: "group", name: target })}
      >
        {t("keys.moveChosen")}
      </Button>
      <Button onClick={onClear}>{t("keys.clearChosen")}</Button>
    </div>
  );
}

// KeyMoveOutcomeNotice は、まとめて移したあとに、移せた数と、移せなかった鍵を
// 1件ずつ理由と一緒に示す。
export function KeyMoveOutcomeNotice({ outcome }: { outcome: MoveOutcome }) {
  const t = useTranslate();
  return (
    <div role="status" className="rounded-lg border border-line bg-card p-3 text-sm">
      <p className="text-ink">{t("keys.moveMoved", { count: outcome.moved.length })}</p>
      {outcome.blocked.map((entry) => (
        <p key={entry.path} className="text-danger">
          {t("keys.moveBlocked", { path: entry.path, reason: entry.blockers.join(" / ") })}
        </p>
      ))}
      {outcome.failed.map((path) => (
        <p key={path} className="text-danger">
          {t("keys.moveFailed", { path })}
        </p>
      ))}
    </div>
  );
}
