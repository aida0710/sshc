import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Icon } from "./icons";
import { Button } from "./surface";
import { useClipboardCopy } from "./useClipboardCopy";

type CopyButtonProps = {
  value: string;
  label: MessageKey;
  // small は、知らせの中や行の端に置く低いボタン。ほかのボタンと並べるときは false にして高さをそろえる。
  small?: boolean;
};

// CopyButton は、値をコピーする字のボタン。書けたら、押したボタンが少しのあいだ ✓ と「コピーしました」に
// 替わる（ボタンが残る所のコピーの知らせ方。閉じるメニューの項目はトーストで知らせる）。
export function CopyButton({ value, label, small = true }: CopyButtonProps) {
  const t = useTranslate();
  const { state, copy } = useClipboardCopy(value);

  return (
    <span className="inline-flex items-center gap-2">
      <Button small={small} onClick={copy} className="inline-flex items-center gap-1.5">
        {state === "copied" ? <><Icon name="check" className="size-3.5" />{t("copy.copied")}</> : t("copy.button", { label: t(label) })}
      </Button>
      {/* ボタンの名前が替わっただけでは読み上げられないので、結果を別に知らせる。断られたことは見える字で残す。 */}
      <span aria-live="polite" className={state === "failed" ? "text-xs text-danger" : "sr-only"}>
        {state === "copied" ? t("copy.done") : state === "failed" ? t("copy.refused") : ""}
      </span>
    </span>
  );
}
