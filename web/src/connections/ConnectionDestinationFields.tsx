import { useId } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import type { BasicFieldState } from "./basicFields";
import { inheritsValue, resetValue, typedValue, type DraftField } from "./basicFormDraft";

type DestinationName = "hostName" | "user" | "port";

type DestinationEntry = {
  draft: DraftField;
  error: string;
  onChange: (next: DraftField) => void;
};

// 継承した値を見本の文字で出すときは、右端の「継承」「既定」の下に見本が入らないよう右を空ける。
const inputWithTail = `${control} pr-9`;

// 戻すボタンは、マウスを載せたときとフォーカスしているときだけ、値の右端に重ねて出す（タッチの画面では
// いつも出す）。いつも出すと、狭い欄で値がボタンの分だけ切れる。入力欄の高さいっぱいに置くので、
// スマホの押す場所の決まり（44px）は data-touch-compact で外す。
const resetButtonLook =
  "absolute inset-y-px right-px flex w-8 items-center justify-center rounded-r-[5px] bg-control text-ink-faint " +
  "opacity-0 transition-opacity hover:bg-hover hover:text-ink focus-visible:opacity-100 " +
  "group-hover/field:opacity-100 group-focus-within/field:opacity-100 mobile-layout:opacity-100 [@media(any-hover:none)]:opacity-100";

const labels: Record<DestinationName, { label: MessageKey; reset: MessageKey }> = {
  hostName: { label: "conn.basicHostName", reset: "conn.basicUseInheritedHost" },
  user: { label: "conn.basicUser", reset: "conn.basicUseInheritedUser" },
  port: { label: "conn.basicPort", reset: "conn.basicUseInheritedPort" },
};

function sourceTitle(state: BasicFieldState, t: ReturnType<typeof useTranslate>): string {
  if (state.origin === "default") return t("conn.basicSSHDefault");
  const path = state.source?.path ?? state.source?.absolute ?? "";
  return t("conn.basicInheritedFrom", { path, line: state.source?.line ?? 0 });
}

function DestinationInput({ name, entry, numeric = false }: { name: DestinationName; entry: DestinationEntry; numeric?: boolean }) {
  const t = useTranslate();
  const id = useId();
  const errorId = useId();
  const sourceId = useId();
  const { draft: field, error, onChange } = entry;
  const complex = field.state.origin === "complex";
  const inherits = !complex && inheritsValue(field);
  // 書いてある値を外したときは、保存したあとに何を継承するかが、この画面には分からない。
  const placeholder = !inherits ? "" : field.inherit ? t("conn.basicInheritPlaceholder") : field.state.value;
  const tail = !inherits || field.inherit ? "" : t(field.state.origin === "default" ? "conn.basicDefaultTail" : "conn.basicInheritedTail");
  const canReset = !inherits && !complex && field.state.editable;
  const warning = complex ? t("conn.basicComplex", { keyword: field.state.keyword }) : error;
  const descriptions = [warning === "" ? "" : errorId, tail === "" ? "" : sourceId].filter((id) => id !== "").join(" ");
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <label htmlFor={id} className="text-xs text-ink-muted">{t(labels[name].label)}</label>
      <span className="group/field relative flex min-w-0">
        <input
          id={id}
          type={numeric ? "number" : "text"}
          {...(numeric ? { min: 1, max: 65535 } : {})}
          value={inherits ? "" : field.value}
          placeholder={placeholder}
          disabled={!field.state.editable || complex}
          aria-invalid={warning === "" ? undefined : true}
          aria-describedby={descriptions === "" ? undefined : descriptions}
          onChange={(event) => onChange(typedValue(field, event.target.value))}
          className={`${tail === "" ? control : inputWithTail} font-mono text-[12.5px]`}
        />
        {tail === "" ? null : (
          <span title={sourceTitle(field.state, t)} className="absolute inset-y-0 right-2.5 flex items-center text-[11.5px] text-ink-faint">
            <span aria-hidden="true">{tail}</span>
            <span id={sourceId} className="sr-only">{sourceTitle(field.state, t)}</span>
          </span>
        )}
        {canReset ? (
          <button
            type="button"
            data-touch-compact
            aria-label={t(labels[name].reset)}
            title={t(labels[name].reset)}
            onClick={() => onChange(resetValue(field))}
            className={resetButtonLook}
          >
            <Icon name="close" className="size-3.5" />
          </button>
        ) : null}
      </span>
      {warning === "" ? null : <span id={errorId} aria-live="polite" className="text-xs leading-[18px] text-notice-ink">{warning}</span>}
    </div>
  );
}

// ConnectionDestinationFields は、基本タブの「接続先」の3つの欄（ホスト名・ユーザー・ポート）である。
// 1行に並べ、狭い欄ではホスト名を1行に、ユーザーとポートを下の1行にする。継承した値と既定値は入力欄の
// 見本の文字で見せ、自分で書いた値には右端に戻すボタンを出す。
export function ConnectionDestinationFields({ hostName, user, port }: Record<DestinationName, DestinationEntry>) {
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_84px] gap-x-2.5 gap-y-2 @sm/editor:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_84px]">
      <div className="col-span-2 @sm/editor:col-span-1">
        <DestinationInput name="hostName" entry={hostName} />
      </div>
      <DestinationInput name="user" entry={user} />
      <DestinationInput name="port" entry={port} numeric />
    </div>
  );
}
