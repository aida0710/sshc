import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { ToastContext, type ShowToast, type ToastTone } from "./toastContext";
import { ToastMessage } from "./ToastMessage";

// 一行の知らせを読み切れて、居座らない長さ。Terminal の知らせと同じにする。
export const toastVisibleMs = 2500;

type VisibleToast = { id: number; message: string; tone: ToastTone };

// ToastProvider は、アプリに一つだけ置き、useToast で出した知らせを画面の下に出す。スマホの
// 浮かせた下タブがあれば、その上に出す。
// 読み上げの領域は常に置いておき、中身を入れ替えて読ませる（出すたびに領域を作ると
// 読み上げられないことがある）。
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<VisibleToast | null>(null);
  const nextId = useRef(0);
  const showToast = useCallback<ShowToast>((message, options) => {
    nextId.current += 1;
    setToast({ id: nextId.current, message, tone: options?.tone ?? "neutral" });
  }, []);

  useEffect(() => {
    if (toast === null) return;
    const timer = window.setTimeout(() => setToast(null), toastVisibleMs);
    return () => window.clearTimeout(timer);
  }, [toast]);

  return (
    <ToastContext.Provider value={showToast}>
      {children}
      <div
        role="status"
        className="pointer-events-none fixed inset-x-0 bottom-[max(calc(1rem+var(--mobile-navigation-inset)),calc(1rem+env(safe-area-inset-bottom,0px)))] z-[80] flex justify-center px-4"
      >
        {toast === null ? null : (
          <ToastMessage key={toast.id} tone={toast.tone} className="max-w-md">
            {toast.message}
          </ToastMessage>
        )}
      </div>
    </ToastContext.Provider>
  );
}
