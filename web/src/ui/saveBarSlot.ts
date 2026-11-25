import { createContext, useState, type RefCallback } from "react";

// SaveBarSlotContext は、ページか欄の下の端にある、保存の帯（DraftSaveBar）を置く要素。フォームがどこにあっても、帯は
// そのページか欄の下の端に固定される。無いところでは、帯はその場に出る。
export const SaveBarSlotContext = createContext<HTMLElement | null>(null);

// useSaveBarSlot は、帯を置く要素を作るページや欄が使う。slotRef を付けた要素（saveBarSlotClass）を、スクロールする
// 中身の最後に置き、slot を SaveBarSlotContext で中身に渡す。
export function useSaveBarSlot(): { slot: HTMLElement | null; slotRef: RefCallback<HTMLElement> } {
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  return { slot, slotRef: setSlot };
}

// saveBarSlotClass は、帯を置く要素のクラス。要素そのものは場所を取らず、帯が中身と同じ縦の並び（flex-col）に入る。
export const saveBarSlotClass = "contents";

// 帯は、置いた並びの左右と下の余白の外まで広がる。余白は並びごとに違うので、並びの側が --save-bar-x（左右の余白）と
// --save-bar-end（下の余白のうち、スマホの下のバーの分を除いたもの）、--save-bar-scroll-end（--save-bar-end のうち、
// スクロールする要素そのものが取っている分）で伝える。sticky はスクロールする要素の余白の内側で止まるので、その分を
// 負の値にして端まで下げる。
//
// 帯の上には線を引かず、続きの中身をぼかす。欄の区切りの線と2本並んで見えるため（利用者、2026-11-25）。ぼかしは帯の箱の
// 上の 1.25rem に描くので、一番下まで送れば最後の行はぼかしの上に出る。ぼかしの下の欄は押せる。
export const saveBarFrame =
  "pointer-events-none [&>*]:pointer-events-auto " +
  "sticky bottom-[calc(-1*(var(--save-bar-scroll-end,0px)+var(--mobile-navigation-inset)))] z-[3] mt-auto " +
  "-mx-(--save-bar-x,0px) -mb-[calc(var(--save-bar-end,0px)+var(--mobile-navigation-inset))] px-(--save-bar-x,0px) " +
  "pt-[1.75rem] pb-[calc(0.625rem+var(--mobile-navigation-inset))] rounded-b-lg mobile-layout:rounded-none " +
  "bg-linear-to-t from-sidebar from-[calc(100%-1.25rem)] to-transparent";

// saveBarScrollRoom は、帯（スマホで64px）とその上のぼかし（20px）の高さ。帯が出ているあいだ、欄を見える所へ送るときに
// 帯の下へ隠れないよう、スクロールする要素の scroll-padding に足す。
export const saveBarScrollRoom = "has-[[data-save-bar]]:scroll-pb-[calc(var(--mobile-navigation-inset)+5.5rem)]";
