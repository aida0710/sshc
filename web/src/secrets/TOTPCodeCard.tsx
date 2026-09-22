import { useCallback, useEffect, useState } from "react";
import type { CredentialsApi, TOTPCodeSet } from "../api/credentials";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";
import { useClipboardCopy } from "../ui/useClipboardCopy";

// 保存済みワンタイムパスワードの、いま有効なコードと、その前後のコード。

function readableCode(code: string): string {
  return code.length === 6 ? `${code.slice(0, 3)} ${code.slice(3)}` : code;
}

// CopyableCode は、コードそのものを押せる範囲にする。使う人が狙うのは数字であり、
// その隣に別の小さなボタンを置くより、数字を押させる方が外さない。
//
// 読みやすさのために挟んだ空白は貼り付け先では邪魔になるので、渡すのは表示では
// なくコードそのものである。
function CopyableCode({ code, label, className }: { code: string; label: string; className?: string }) {
  const t = useTranslate();
  const { state, copy } = useClipboardCopy(code);
  const refused = state === "failed";
  return (
    <button
      type="button"
      aria-label={label}
      title={refused ? t("copy.refused") : label}
      onClick={copy}
      className={`group -ml-2 flex items-center gap-2 rounded-md px-2 py-1 text-left hover:bg-select-fill ${className ?? ""}`}
    >
      <span className="whitespace-nowrap font-mono tracking-wider">{readableCode(code)}</span>
      <Icon
        name={state === "copied" ? "check" : refused ? "close" : "copy"}
        className={`size-4 ${refused ? "text-danger" : "text-ink-muted group-hover:text-ink"}`}
      />
      <span aria-live="polite" className="sr-only">
        {state === "copied" ? t("copy.done") : refused ? t("copy.refused") : ""}
      </span>
    </button>
  );
}

export function TOTPCodeCard({ name, api }: { name: string; api: Pick<CredentialsApi, "totpCodes"> }) {
  const t = useTranslate();
  const [codes, setCodes] = useState<TOTPCodeSet | null>(null);
  const [remaining, setRemaining] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const [failed, setFailed] = useState(false);

  const reload = useCallback(async () => {
    try {
      const next = await api.totpCodes(name);
      setCodes(next);
      setRemaining(next.remainingSeconds);
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, [api, name]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (codes === null) return;
    const timer = window.setInterval(() => {
      setRemaining((current) => {
        if (current > 1) return current - 1;
        void reload();
        return 0;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [codes, reload]);

  if (failed) {
    return <Button onClick={() => void reload()}>{t("secrets.totpRetry")}</Button>;
  }
  if (codes === null) {
    return <p className={hintText}>{t("secrets.totpLoading")}</p>;
  }
  return (
    <div className="min-w-0 rounded-md bg-surface-subtle px-3 py-2 sm:min-w-72">
      <div className="flex items-center gap-3">
        <CopyableCode
          code={codes.current}
          label={t("secrets.totpCopyCurrent", { name })}
          className="min-w-0 flex-1 text-lg font-semibold text-ink"
        />
        <span className="shrink-0 text-xs tabular-nums text-ink-muted">
          {t("secrets.totpRemaining", { seconds: remaining })}
        </span>
        <button
          type="button"
          aria-expanded={expanded}
          aria-label={t(expanded ? "secrets.totpCollapse" : "secrets.totpExpand", { name })}
          className="flex size-8 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill hover:text-ink"
          onClick={() => setExpanded((current) => !current)}
        >
          <Icon name="chevronRight" className={`size-4 transition-transform ${expanded ? "rotate-90" : ""}`} />
        </button>
      </div>
      {expanded ? (
        <div className="mt-2 grid grid-cols-2 gap-2 border-t border-line pt-2 text-xs">
          <div>
            <p className="text-ink-faint">{t("secrets.totpPrevious")}</p>
            <CopyableCode
              code={codes.previous}
              label={t("secrets.totpCopyPrevious", { name })}
              className="mt-1 text-ink-muted"
            />
          </div>
          <div>
            <p className="text-ink-faint">{t("secrets.totpNext")}</p>
            <CopyableCode
              code={codes.next}
              label={t("secrets.totpCopyNext", { name })}
              className="mt-1 text-ink-muted"
            />
          </div>
        </div>
      ) : null}
    </div>
  );
}
