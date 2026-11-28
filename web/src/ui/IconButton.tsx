import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";

// IconButton の大きさ。PC の組み方の大きさで、今いちばん多く使われている大きさだけを置く。スマホの組み方では、
// index.css の決まりがどのボタンも 44px にする（"row" は data-touch-dense で高さ 36px）ので、ここでは決めない。
// "bar" と "barLarge" は、スマホの組み方でしか出ない上のバーや SFTP の操作の帯のボタン。
const sizes = {
  // 詰めた一覧の行の中（サイドバーのセッションとワークスペースの行）。
  row: "size-5",
  // 行の中の小さな操作、知らせの［×］。
  compact: "size-6",
  // 道具の帯、行の右端の操作。
  toolbar: "size-7",
  // 板やダイアログの見出しの操作と［閉じる］、カードの「…」。
  panel: "size-8",
  // 一覧の行の「…」のうち、行が高いもの。
  large: "size-9",
  // SFTP のタブの中と横の操作。タブの高さ（30px）に合わせ、タブの字と並ぶようにする。
  tab: "h-[30px] w-6",
  tabWide: "h-[30px] w-7",
  bar: "size-11",
  barLarge: "size-12",
} as const;

// 字の色。"faint" は目立たせない操作、"danger" は消す操作（載せたときに赤）。
const tones = {
  muted: "text-ink-muted hover:text-ink",
  faint: "text-ink-faint hover:text-ink",
  danger: "text-ink-faint hover:text-danger",
  // plain は字の色を付けない。色で状態を示すボタン（止めている、警告がある）が className で付ける。
  plain: "",
} as const;

// 載せたときの地。置く面ごとに見えやすい地が違う。"none" は地を付けない（押した印は index.css の :active が付ける）。
const grounds = {
  hover: "hover:bg-hover",
  surface: "hover:bg-surface",
  card: "hover:bg-card/50",
  none: "",
} as const;

export type IconButtonSize = keyof typeof sizes;
export type IconButtonTone = keyof typeof tones;
export type IconButtonGround = keyof typeof grounds;

export type IconButtonProps = {
  // label は、読み上げの名前と、載せたときの吹き出し。字の無いボタンなので必ず要る。
  label: string;
  size: IconButtonSize;
  tone?: IconButtonTone;
  ground?: IconButtonGround;
  // dense は、詰めた一覧の行の中のボタン。スマホの組み方で高さを 36px にする（index.css の data-touch-dense）。
  dense?: boolean;
  // touchCompact は、入力欄の中に重ねるボタンのように、スマホでも描いた大きさのままにするもの（data-touch-compact）。
  touchCompact?: boolean;
  // children は印（Icon）。印の大きさは置く場所の字に合わせてあるので、呼ぶ側が決める。
  children: ReactNode;
} & Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label" | "title" | "children">;

// IconButton は、字が無く印だけのボタン（［×］、「…」、検索、更新など）の1つの形。形（角の丸め、真ん中に置いた印、
// 字の色、載せたときの地）と大きさを1か所で決める。行に合わせた見せ方（乗せたときだけ出す）や、状態の色
// （止めている、開いている）は className で足す。
export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { label, size, tone = "muted", ground = "hover", dense = false, touchCompact = false, className = "", type = "button", children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      aria-label={label}
      title={label}
      {...(dense ? { "data-touch-dense": "" } : {})}
      {...(touchCompact ? { "data-touch-compact": "" } : {})}
      className={`flex shrink-0 items-center justify-center rounded-md transition-colors duration-150 ${sizes[size]} ${tones[tone]} ${grounds[ground]} ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
});
