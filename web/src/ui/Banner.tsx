import type { ReactNode } from "react";

// BannerTone は帯の地と線。地の色は帯ごとに決まっていて、そろえない（知らせの地を1つにしたら、前の色のほうが
// 好まれた）。
// - notice: 待っていること・確かめてほしいこと（琥珀）。ターミナルの再接続中、区画の知らせ。
// - quiet: 終わったことの知らせ（カードの地）。終わったセッション。字の色は中身が決める（正常に終われば淡く、
//   失敗なら赤）。
// - danger: 失敗の知らせ（薄い赤の地）。字の色は中身が決める。
// - plain: 地を付けず、下に線だけを引く。プレビューの上の失敗の1行。
// - overlay: 写真や動画の上に重ねた暗い見出しの中。地も線も付けない（高さは線のある帯と同じ）。
export type BannerTone = "notice" | "quiet" | "danger" | "plain" | "overlay";

const bannerTones: Record<BannerTone, string> = {
  notice: "border-notice-line bg-notice text-notice-ink",
  quiet: "border-line bg-card",
  danger: "border-danger/40 bg-danger/5",
  plain: "border-line",
  overlay: "border-transparent",
};

// bannerButton は、帯に置くボタン（Button の small）に足す。スマホの組み方では、指で押せるように少し高くする。
export const bannerButton = "mobile-layout:min-h-8";

// Banner は、区画の上の端に出す細い帯。ターミナルの上の帯、画面の上のエラーの帯、区画の知らせ、プレビューの失敗が
// 使う。文を左に、ボタンを右にまとめる。文は 12rem を下回るまで縮まず、それより狭いとボタンを次の行へ送る。
// multiline は、見出しと説明のように中身が何行にもなる帯で、ボタンを上の端にそろえる。details は、文とボタンの行の
// 下に帯の幅いっぱいで置く中身（エラーの帯の診断情報）。文の列に入れると、スマホではボタンの幅だけ細くなる。
// labelledBy を渡すと、帯をその見出しの名前の区画（section）にする。
export function Banner({ tone, role, labelledBy, actions, multiline = false, details, children }: {
  tone: BannerTone;
  role?: "status" | "alert" | undefined;
  labelledBy?: string;
  actions?: ReactNode;
  multiline?: boolean;
  details?: ReactNode;
  children: ReactNode;
}) {
  const Frame = labelledBy === undefined ? "div" : "section";
  return (
    <Frame
      role={role}
      aria-labelledby={labelledBy}
      className={`flex shrink-0 flex-wrap ${multiline ? "items-start" : "items-center"} gap-x-3 gap-y-1.5 border-b px-3 py-1.5 text-xs ${bannerTones[tone]}`}
    >
      <div className="min-w-0 flex-[1_1_12rem]">{children}</div>
      {actions === undefined ? null : <div className="ml-auto flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      {details === undefined ? null : <div className="basis-full">{details}</div>}
    </Frame>
  );
}
