import type { ReactNode } from "react";
import { Icon } from "./icons";
import type { ToastTone } from "./toastContext";

// ToastMessage は、短い知らせ一つの見た目である。置く場所（画面の下、ターミナルの右下）は
// 呼び手が className で決める。押せる物を覆わないよう、ポインタの操作は受けない。
export function ToastMessage({
  tone = "neutral",
  role,
  className = "",
  children,
}: {
  tone?: ToastTone;
  role?: "status";
  className?: string;
  children: ReactNode;
}) {
  return (
    <p
      role={role}
      className={`pointer-events-none flex items-center gap-2 rounded border border-line bg-toolbar/95 px-3 py-2 text-xs text-ink shadow-float animate-[sshc-dialog-enter_150ms_ease-out] ${className}`}
    >
      {tone === "success" ? <Icon name="check" className="size-3.5 shrink-0 text-live" /> : null}
      {children}
    </p>
  );
}
