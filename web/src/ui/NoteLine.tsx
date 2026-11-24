import type { ReactNode } from "react";
import { Icon } from "./icons";

const noteLineMarks = {
  notice: { icon: "warning", colour: "text-notice-ink" },
  danger: { icon: "failure", colour: "text-danger" },
} as const;

// NoteLine は、入力欄、設定の行、一覧の行の下に添える1行の注意（"notice"）か失敗（"danger"）。枠に入れるほどでは
// ない短い文なので、枠を付けずに、意味の色の印と墨の字で書く（知らせのカードと同じく、意味の色を字に使わない）。
// 印の形も注意と失敗で変え、色だけで見分けなくてよいようにする。
export function NoteLine({ tone = "notice", id, live = true, className = "", children }: {
  tone?: keyof typeof noteLineMarks;
  // id は、入力欄が aria-describedby でこの文を指すときに付ける。
  id?: string | undefined;
  // live は、出たときに読み上げるか。入力欄の説明として読まれる文や、最初から一覧にある文は false にする。
  live?: boolean;
  className?: string;
  children: ReactNode;
}) {
  const mark = noteLineMarks[tone];
  return (
    <p
      id={id}
      role={!live ? undefined : tone === "danger" ? "alert" : "status"}
      className={`flex gap-1.5 text-xs leading-[18px] text-ink ${className}`}
    >
      <Icon name={mark.icon} className={`mt-0.5 size-3.5 shrink-0 ${mark.colour}`} />
      <span className="min-w-0 break-words">{children}</span>
    </p>
  );
}
