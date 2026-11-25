import type { ReactNode } from "react";

// ActionPlacement は、ボタンの行を置く場所。線を引くかは置き場所で決まる（2026-11-25、ダイアログやフォームの下の
// ボタンの行が、線の有無・寄せ方・余白でそれぞれ違っていたのをそろえた）。
// - page: ページのフォームの最後。デザインのとおり上に線を引き、上の節と間をあける。
// - inline: ダイアログや、その場で開く小さな欄の中。中身と一緒に流れるので、線は引かない。
// - band: 中身がスクロールするダイアログの下に固定する帯。スクロールする中身と分けるため、上に線を引く。余白は
//   同じダイアログの見出しの帯とそろえるので、呼び手が className で渡す。
export type ActionPlacement = "page" | "inline" | "band";

const placementFrames: Record<ActionPlacement, string> = {
  page: "mt-6 min-h-12 border-t border-line pt-4",
  inline: "",
  band: "shrink-0 border-t border-line",
};

// ActionArea は、フォームやダイアログの最後のボタンの行。押すものを右に寄せ、狭い幅では端からはみ出さずに
// 折り返す。start は左に置くもの（何が起きたかの文や、ほかの操作）。ボタンの順と最初のフォーカスは呼び手が決める。
export function ActionArea({ children, start, placement = "page", className = "" }: {
  children: ReactNode;
  start?: ReactNode;
  placement?: ActionPlacement;
  className?: string;
}) {
  return (
    <div className={`flex flex-wrap items-center justify-between gap-3 ${placementFrames[placement]} ${className}`}>
      {/* 左のものは 16rem を下回るまで縮まず、それより狭いとボタンを次の行へ送る（スマホで文が細い柱にならないように）。 */}
      {start === undefined ? null : <div className="min-w-0 flex-[1_1_16rem]">{start}</div>}
      <div className="ml-auto flex max-w-full shrink-0 flex-wrap justify-end gap-2">{children}</div>
    </div>
  );
}
