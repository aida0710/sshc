import type { MarkedProgramState } from "./programStatus";

// statusMarkCircle は、セッションの行の先頭の印（接続の点、状態の印、進み具合の輪）の丸である。大きさを1つに
// そろえ、状態が点に代わっても行の名前が動かないようにする。
export const statusMarkCircle = "size-[11px] rounded-full";

// 状態ごとの、塗った丸の色と中に描く記号。
const marks: Record<MarkedProgramState, { fill: string; sign: string }> = {
  blocked: { fill: "bg-notice-ink shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-notice-ink)_22%,transparent)]", sign: "!" },
  error: { fill: "bg-danger", sign: "×" },
  done: { fill: "bg-live", sign: "✓" },
};

// ProgramStatusMark は、プログラムの状態（OSC 7501）の入力待ち・失敗・完了の印である。セッションの行の先頭と、
// ナビゲーションを開くボタンの角が同じ印を出す。同じことを文や名前が言うので、読み上げからは隠す。置き場所は
// className で決める。
export function ProgramStatusMark({ state, className }: { state: MarkedProgramState; className: string }) {
  const mark = marks[state];
  return (
    <span aria-hidden="true" data-program-status={state}
      className={`${statusMarkCircle} flex items-center justify-center text-[8px] font-bold leading-none text-canvas ${mark.fill} ${className}`}>
      {mark.sign}
    </span>
  );
}
