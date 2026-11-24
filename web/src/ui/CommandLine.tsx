import { useTranslate } from "../i18n/context";
import { Icon } from "./icons";
import { useClipboardCopy } from "./useClipboardCopy";

// CommandLine は、利用者がほかのマシンで打つコマンドを1行で見せ、そのままコピーできるようにする。
// note は、そのコマンドで何が起きるかの短い説明。ブラウザがコピーを断ったときは、note の代わりにそのことを書く。
export function CommandLine({ command, note }: { command: string; note: string }) {
  const t = useTranslate();
  const { state, copy } = useClipboardCopy(command);
  const label = state === "copied" ? t("copy.done") : state === "failed" ? t("copy.refused") : t("copy.button", { label: command });
  return (
    <div className="flex items-center gap-1 rounded-md border border-line bg-well py-1 pl-2 pr-1">
      <span className="flex min-w-0 grow flex-col">
        <code className="break-all font-mono text-xs leading-5 text-ink">{command}</code>
        <span className={`text-2xs leading-4 ${state === "failed" ? "text-danger" : "text-ink-muted"}`}>
          {state === "failed" ? t("copy.refused") : note}
        </span>
      </span>
      <button
        type="button"
        aria-label={label}
        title={label}
        onClick={copy}
        className="flex size-7 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
      >
        <Icon name={state === "copied" ? "check" : "copy"} className="size-3.5" />
      </button>
      {/* 名前が変わっただけでは読み上げられないので、結果を別に知らせる。 */}
      <span aria-live="polite" className="sr-only">{state === "copied" ? t("copy.done") : state === "failed" ? t("copy.refused") : ""}</span>
    </div>
  );
}
