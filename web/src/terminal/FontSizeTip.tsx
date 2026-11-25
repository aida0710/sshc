import { useTranslate } from "../i18n/context";
import type { FontSizeTip as FontSizeTipState } from "./useFontSizeTip";

type Props = {
  tip: FontSizeTipState;
  // size は今の文字の大きさ、settingsSize は設定が決めた大きさである（どちらも CSS ピクセル）。
  size: number;
  settingsSize: number;
  onReset: () => void;
};

// FontSizeTip は、ターミナルの文字の大きさを変えた直後に、今の大きさと、設定の大きさへ戻すボタンを右上に出す。
// 設定の大きさと同じなら、戻す先が無いのでボタンは出さない。
export function FontSizeTip({ tip, size, settingsSize, onReset }: Props) {
  const t = useTranslate();
  if (!tip.shown) return null;
  const zoomed = size !== settingsSize;
  return (
    <div
      role="status"
      className={`absolute right-3 top-3 z-20 flex items-center gap-2 rounded-md border border-line bg-toolbar/95 py-1 pl-3 text-xs text-ink shadow-float animate-[sshc-dialog-enter_150ms_ease-out] ${zoomed ? "pr-1" : "pr-3"}`}
      onPointerEnter={tip.hold}
      onPointerLeave={tip.release}
    >
      <span className="py-1 font-mono tabular-nums">{t("terminal.fontSizeTipSize", { size })}</span>
      {!zoomed ? null : (
        <button
          type="button"
          className="rounded-sm px-2 py-1 text-ink-muted hover:bg-hover hover:text-ink mobile-layout:min-h-11 mobile-layout:px-3"
          title={t("terminal.fontSizeTipResetTitle", { size: settingsSize })}
          // 押してもフォーカスを取らない。ターミナルで打っている途中の人のキーの行き先を変えず、スマホでは
          // キーボードを開閉させない。
          onPointerDown={(event) => event.preventDefault()}
          onMouseDown={(event) => event.preventDefault()}
          onClick={onReset}
        >
          {t("terminal.fontSizeTipReset")}
        </button>
      )}
    </div>
  );
}
