import type { TOTPCodeSet } from "../api/credentials";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";
import { useClipboardCopy } from "../ui/useClipboardCopy";
import type { TOTPCodes } from "./useTOTPCodes";

// 保存済みワンタイムパスワードの、いま有効なコードと、その前後のコード。

function readableCode(code: string): string {
  return code.length === 6 ? `${code.slice(0, 3)} ${code.slice(3)}` : code;
}

// CopyableCode は、コードそのものを押せる範囲にする。使う人が狙うのは数字であり、
// その隣に別の小さなボタンを置くより、数字を押させる方が外さない。
//
// 読みやすさのために挟んだ空白は貼り付け先では邪魔になるので、渡すのは表示では
// なくコードそのものである。
function CopyableCode({ code, label, className }: { code: string; label: string; className: string }) {
  const t = useTranslate();
  const { state, copy } = useClipboardCopy(code);
  const refused = state === "failed";
  return (
    <button
      type="button"
      aria-label={label}
      title={refused ? t("copy.refused") : label}
      onClick={copy}
      className={`flex items-center gap-1.5 rounded-md px-1.5 py-0.5 hover:bg-select-fill ${className}`}
    >
      <span className="whitespace-nowrap font-mono tabular-nums tracking-wider">{readableCode(code)}</span>
      {state === "idle" ? null : <Icon name={state === "copied" ? "check" : "close"} className={`size-4 ${refused ? "text-danger" : "text-ink-muted"}`} />}
      <span aria-live="polite" className="sr-only">
        {state === "copied" ? t("copy.done") : refused ? t("copy.refused") : ""}
      </span>
    </button>
  );
}

// The ring empties as the current code's time runs out, with the seconds left inside it.
const ringSize = 26;
const ringStroke = 3;

function CountdownRing({ remaining, period, label }: { remaining: number; period: number; label: string }) {
  const radius = ringSize / 2 - ringStroke / 2 - 1;
  const circumference = 2 * Math.PI * radius;
  const share = period > 0 ? Math.min(1, Math.max(0, remaining / period)) : 0;
  return (
    <span role="timer" aria-label={label} className="relative inline-flex size-[26px] shrink-0 items-center justify-center">
      <svg aria-hidden="true" width={ringSize} height={ringSize} className="absolute inset-0 -rotate-90">
        <circle cx={ringSize / 2} cy={ringSize / 2} r={radius} fill="none" strokeWidth={ringStroke} className="stroke-well" />
        <circle
          cx={ringSize / 2}
          cy={ringSize / 2}
          r={radius}
          fill="none"
          strokeWidth={ringStroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={circumference * (1 - share)}
          className="stroke-accent transition-[stroke-dashoffset] duration-1000 ease-linear motion-reduce:transition-none"
        />
      </svg>
      <span aria-hidden="true" className="font-mono text-[10px] font-semibold tabular-nums text-ink-muted">{remaining}</span>
    </span>
  );
}

// TOTPCurrentCode is the code a row shows: the current one, large enough to read at a glance, and its ring.
export function TOTPCurrentCode({ name, codes }: { name: string; codes: TOTPCodes }) {
  const t = useTranslate();
  if (codes.state === "failed") return <Button onClick={codes.retry}>{t("secrets.totpRetry")}</Button>;
  if (codes.state === "loading") return <p className={hintText}>{t("secrets.totpLoading")}</p>;
  return (
    <span className="flex items-center gap-2">
      <CopyableCode code={codes.codes.current} label={t("secrets.totpCopyCurrent", { name })} className="text-[22px] font-semibold leading-7 text-ink" />
      <CountdownRing
        remaining={codes.remainingSeconds}
        period={codes.codes.periodSeconds}
        label={t("secrets.totpRemaining", { seconds: codes.remainingSeconds })}
      />
    </span>
  );
}

// TOTPNeighbourCodes is what the row's menu opens under it: the codes before and after the current one, for
// a server whose clock is a step off.
export function TOTPNeighbourCodes({ name, codes }: { name: string; codes: TOTPCodeSet }) {
  const t = useTranslate();
  return (
    <div className="flex gap-6 rounded-lg bg-well px-3 py-2 text-xs text-ink-faint">
      <div>
        <p>{t("secrets.totpPrevious")}</p>
        <CopyableCode code={codes.previous} label={t("secrets.totpCopyPrevious", { name })} className="-ml-1.5 mt-0.5 text-[15px] font-semibold text-ink-muted" />
      </div>
      <div>
        <p>{t("secrets.totpNext")}</p>
        <CopyableCode code={codes.next} label={t("secrets.totpCopyNext", { name })} className="-ml-1.5 mt-0.5 text-[15px] font-semibold text-ink-muted" />
      </div>
    </div>
  );
}
