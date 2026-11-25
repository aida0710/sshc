import {
  forwardRef,
  type ButtonHTMLAttributes,
  type ElementType,
  type HTMLAttributes,
  type ReactNode,
  useId,
} from "react";
import { useTranslate } from "../i18n/context";
import { describeControl } from "./describeControl";
import { DisclosureSummary } from "./DisclosureSummary";
import { Icon } from "./icons";
import { dangerAction, dangerTextAction, hintText, primaryAction, secondaryAction, smallSecondaryAction } from "./form";
import { segmentClass, segmentedStrip } from "./segmentedStyle";
import { Spinner } from "./Spinner";
import { IconButton } from "./IconButton";

type CardElement = "div" | "section" | "article" | "dl" | "ul";

type CardProps = {
  as?: CardElement;
  padded?: boolean;
  tone?: "card" | "notice" | "subtle";
  overflow?: "hidden" | "visible" | "x-auto";
} & HTMLAttributes<HTMLElement>;

export function Card({
  as,
  children,
  padded = false,
  tone = "card",
  overflow = "hidden",
  className = "",
  ...rest
}: CardProps) {
  const Component = (as ?? "div") as ElementType;
  const toneClass = tone === "notice" ? "bg-notice" : tone === "subtle" ? "bg-surface-subtle" : "bg-card";
  const overflowClass = overflow === "visible" ? "overflow-visible" : overflow === "x-auto" ? "overflow-x-auto" : "overflow-hidden";
  return (
    <Component
      className={`sshc-card ${overflowClass} ${toneClass} ${
        padded ? "flex flex-col gap-3 p-3" : ""
      } ${className}`}
      {...rest}
    >
      {children}
    </Component>
  );
}

export function Row({
  label,
  children,
  hint,
  warning,
  action,
  stackOnNarrow = false,
  interactiveChildren = false,
}: {
  label: string;
  children: ReactNode;
  hint?: string | undefined;
  warning?: string | undefined;
  action?: ReactNode;
  stackOnNarrow?: boolean;
  interactiveChildren?: boolean;
}) {
  const hintId = useId();
  const rowLayout = stackOnNarrow
    ? "flex flex-col items-stretch gap-2 px-3 py-3 sm:flex-row sm:items-center sm:gap-3 sm:py-2"
    : "flex items-center gap-3 px-3 py-2";
  const labelLayout = stackOnNarrow
    ? "flex min-w-0 flex-1 flex-col items-stretch gap-1.5 sm:flex-row sm:items-center sm:gap-3"
    : "flex min-w-0 flex-1 items-center gap-3";
  const contents = (
    <>
      <span
        className={`${stackOnNarrow ? "w-full sm:w-32" : "w-32"} shrink-0 text-sm text-ink-muted`}
      >
        {label}
      </span>
      <span
        className={`${stackOnNarrow ? "w-full sm:w-auto" : "ml-auto"} flex min-w-0 flex-1 justify-end`}
      >
        {describeControl(children, { descriptionIds: [hint === undefined ? undefined : hintId] })}
      </span>
    </>
  );
  return (
    <div className="border-t border-hairline first:border-t-0">
      <div className={rowLayout}>
        {interactiveChildren ? (
          <div className={labelLayout}>{contents}</div>
        ) : (
          <label className={labelLayout}>{contents}</label>
        )}
        {action === undefined ? null : (
          <span className="shrink-0">{action}</span>
        )}
      </div>
      {hint === undefined ? null : (
        <p id={hintId} className={`px-3 pb-2 ${hintText}`}>{hint}</p>
      )}
      {warning === undefined ? null : (
        <p role="status" className="px-3 pb-2 text-xs text-notice-ink">
          {warning}
        </p>
      )}
    </div>
  );
}

// 知らせの枠と地。地は前からのもの（注意は黄色、失敗は地なし、済んだことは薄い緑）にする。ターミナルの知らせの枠の、
// カードの色に意味の色を混ぜた地は、画面の中に置くと前の地より好まれなかった（2026-11-24、利用者）。
// 失敗は地が無いので、枠の線を赤にして見分ける。
const noticeToneFrames = {
  notice: "border-notice-line bg-notice",
  danger: "border-danger/45",
  success: "border-live/40 bg-live/10",
} as const;

// Notice は、画面の中で、何かがうまくいかなかったこと、気にかけてほしいこと、今済んだこと（"success": 保存した、
// 始めた、戻した）を知らせるただ1つの形。意味の色は枠と地に付け、字は墨で読ませる（赤や琥珀の字の段落は読みにくい）。
// "compact" は、詰まった一覧やサイドの区画に収める大きさ。
// 1文で足りる知らせは children だけにする。2文以上になるときは、何が起きたかを title に、そのあとを children に書く。
// details は、読んですぐには要らないもの（直し方、エラーの番号）で、最初は畳んでおく。常に見える長い説明を置かないため。
// actions は、知らせについて取れる操作で、文の下、枠の中に置く。長い文が横から押し縮められないようにするため。
export function Notice({
  children,
  title,
  titleLevel,
  tone = "notice",
  compact = false,
  details,
  actions,
  onDismiss,
  id,
  live = true,
}: {
  children?: ReactNode;
  title?: string | undefined;
  // titleLevel は、title を見出し（h3〜h5）にし、知らせをその見出しの節にする。一覧や入力欄を持ち、ページの中で
  // 節のように読ませる箱（実行される設定の一覧、手で確かめる鍵など）に付ける。
  titleLevel?: 3 | 4 | 5 | undefined;
  tone?: keyof typeof noticeToneFrames;
  compact?: boolean;
  details?: { label: string; content: ReactNode } | undefined;
  actions?: ReactNode;
  // onDismiss は、知らせを消す。失敗した操作の結果のように、次の操作まで残る知らせに付け、右上に［×］を出す。
  // 今の状態を表す知らせ（読めない、ロック中など）には付けない。状態が変われば消えるため。
  onDismiss?: (() => void) | undefined;
  // id は、ダイアログや入力欄がこの知らせを説明として指すときに付ける。
  id?: string | undefined;
  // live は、出たときに読み上げるか。最初から置いてある注意の箱や、ダイアログの説明として読まれる文は false にする。
  live?: boolean;
}) {
  const t = useTranslate();
  const titleId = useId();
  const spacing = compact ? "px-2 py-1.5 text-xs" : "px-3 py-2 text-sm";
  const below = compact ? "mt-1.5" : "mt-2";
  const Frame = titleLevel === undefined ? "div" : "section";
  const Title = titleLevel === undefined ? "p" : (`h${titleLevel}` as const);
  return (
    <Frame
      id={id}
      role={!live ? undefined : tone === "danger" ? "alert" : "status"}
      aria-labelledby={titleLevel === undefined ? undefined : titleId}
      className={`rounded-inset border text-ink ${spacing} ${noticeToneFrames[tone]}`}
    >
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1 break-words">
          {title === undefined ? null : <Title id={titleId} className="font-semibold">{title}</Title>}
          {title === undefined || children === undefined ? children : <div className="text-ink-muted">{children}</div>}
        </div>
        {onDismiss === undefined ? null : (
          <IconButton label={t("terminal.noticeDismiss")} size="compact" className="-my-1 -mr-1 ml-auto self-start"
            onClick={onDismiss}>
            <Icon name="close" className="size-3.5" />
          </IconButton>
        )}
      </div>
      {details === undefined ? null : (
        <details className={below}>
          <DisclosureSummary className="text-ink-muted">{details.label}</DisclosureSummary>
          <div className={below}>{details.content}</div>
        </details>
      )}
      {actions === undefined ? null : <div className={`flex flex-wrap items-center gap-2 ${below}`}>{actions}</div>}
    </Frame>
  );
}

// touchCompact keeps the segments their drawn size on a phone, where buttons are otherwise drawn 44px tall; a
// transparent band above and below each still takes a 44px press.
export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
  touchCompact = false,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
  touchCompact?: boolean;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className={segmentedStrip}
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          {...(touchCompact ? { "data-touch-compact": "" } : {})}
          className={`${segmentClass(value === option.value)} ${touchCompact ? "relative after:absolute after:inset-x-0 after:-inset-y-[9px] after:content-['']" : ""}`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

type ButtonProps = (
  // small は知らせの中や状態の行の端に置く低いボタン。今は secondary だけにある。
  | { kind?: "secondary"; small?: boolean }
  | { kind: "primary" | "danger" | "dangerText"; small?: never }
) & {
  // busy は、このボタンで始めた処理を待っている間。回転の印を添え、aria-busy を付けて押せなくする
  // （薄くはしない。index.css の決まり）。busyLabel は待っている間の文言（「保存中…」）で、あれば文言を替える。
  busy?: boolean;
  busyLabel?: string;
} & ButtonHTMLAttributes<HTMLButtonElement>;

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    kind = "secondary",
    small = false,
    busy = false,
    busyLabel,
    disabled,
    className = "",
    type = "button",
    children,
    ...rest
  },
  ref,
) {
  const base =
    kind === "primary"
      ? primaryAction
      : kind === "danger"
        ? dangerAction
        : kind === "dangerText"
          ? dangerTextAction
          : small
            ? smallSecondaryAction
            : secondaryAction;
  // 回転の印と文言を横に並べるため、待っている間だけ flex にする。いつも flex にすると、
  // 幅いっぱいに広げたボタンの文言が左に寄る。
  const busyLayout = busy ? "inline-flex items-center justify-center gap-2" : "";
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled === true || busy}
      aria-busy={busy || undefined}
      className={`${base} ${busyLayout} ${className}`}
      {...rest}
    >
      {busy ? <Spinner /> : null}
      {busy && busyLabel !== undefined ? busyLabel : children}
    </button>
  );
});
