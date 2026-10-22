import { createContext } from "react";

export type ToastTone = "neutral" | "success";

// ShowToast は、画面の下に短い知らせを一つ出す。前の知らせが残っていれば置き換える。
export type ShowToast = (message: string, options?: { tone?: ToastTone }) => void;

export const ToastContext = createContext<ShowToast | null>(null);
