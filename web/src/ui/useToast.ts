import { useContext } from "react";
import { ToastContext, type ShowToast } from "./toastContext";

const ignoreToast: ShowToast = () => undefined;

// useToast は、ToastProvider の知らせを出す関数を返す。ダイアログを閉じたあとなど、
// 操作した場所に結果を書く余地が無いときに、終わったことを伝えるのに使う。
// ToastProvider の外（部品だけを描く単体テスト）では何もしない。
export function useToast(): ShowToast {
  return useContext(ToastContext) ?? ignoreToast;
}
