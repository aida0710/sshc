import { useState, type ReactNode } from "react";
import { Icon } from "./icons";
import { useTranslate } from "../i18n/context";

// ペインの中の 1 面。key はセグメントの選択を保つための識別子で、訳さない。
export type InspectorPane = { key: string; label: string; body: ReactNode };

// セクションが右側のペインに何を置くか、そしてそこにあるものが
// 注意を必要とするかどうか。null を渡すセクションにはトグルすら付かない:
// どこにでも提供されるのに 10 回のうち 9 回は空のペインは、人々にそれを
// 開かないよう教え込んでしまう。
//
// header は panes の上に固定され、セグメントで切り替わらない。いま開いている
// ものが何かは、どちらの面を見ているかによらず見えていなければならない。
// panes が 1 枚のときセグメントは描かれないので、1 面しか持たないセクションの
// 見た目はこれが増える前と変わらない。
export type InspectorContent = {
  label: string;
  attention: boolean;
  header?: ReactNode;
  panes: InspectorPane[];
} | null;

export const inspectorId = "inspector";

export function InspectorToggle({
  label,
  open,
  attention,
  onToggle,
}: {
  label: string;
  open: boolean;
  attention: boolean;
  onToggle: () => void;
}) {
  const t = useTranslate();
  const action = t(open ? "shell.inspectorHideNamed" : "shell.inspectorShowNamed", { label });
  // 名前は 2 つの sr-only span から組み立てるのではなく、ここに直接書く。
  // 隣接する span はセパレータなしで連結され——「Show detailsNeeds
  // attention」——になる。両者の間にあるのは aria-hidden なアイコンだけで、
  // JSX がその周りの空白を取り除いてしまうからだ。
  const name = attention ? `${action} ${t("shell.inspectorAttention")}` : action;
  return (
    <button
      type="button"
      aria-label={name}
      aria-expanded={open}
      aria-controls={inspectorId}
      onClick={onToggle}
      className={`relative flex items-center gap-1.5 rounded-md border border-control-line px-2 py-1 text-ink ${
        open ? "bg-select-fill" : "bg-card"
      }`}
    >
      <Icon name="inspector" className="h-4 w-4" />
      <span className="hidden max-w-44 truncate text-xs sm:inline">{label}</span>
      {/* ドットは目のためのもので、上の文はそれ以外のすべての人のためのものだ。 */}
      {attention ? (
        <span
          aria-hidden="true"
          className="absolute -right-1 -top-1 h-2 w-2 rounded-full border border-toolbar bg-notice-ink"
        />
      ) : null}
    </button>
  );
}

export function InspectorPane({ label, content }: { label: string; content: InspectorContent }) {
  const [active, setActive] = useState<string | null>(null);
  if (content === null) return null;

  // 選ばれていた面が消えた場合は先頭へ戻る。接続を閉じると「設定」が無くなる。
  const current = content.panes.find((pane) => pane.key === active) ?? content.panes[0];

  return (
    <aside
      id={inspectorId}
      aria-label={label}
      className="relative flex flex-col overflow-y-auto border-l border-line bg-sidebar p-3"
    >
      {content.header === undefined ? null : <div className="mb-3 shrink-0">{content.header}</div>}
      {content.panes.length > 1 ? (
        <div
          role="tablist"
          aria-label={label}
          className="mb-3 grid shrink-0 grid-flow-col rounded-lg border border-control-line bg-control p-0.5"
        >
          {content.panes.map((pane) => (
            <button
              key={pane.key}
              type="button"
              role="tab"
              aria-selected={pane.key === current?.key}
              onClick={() => setActive(pane.key)}
              className={`rounded-md px-2 py-1 text-xs ${
                pane.key === current?.key ? "bg-card text-ink shadow-sm" : "text-ink-muted"
              }`}
            >
              {pane.label}
            </button>
          ))}
        </div>
      ) : null}
      <div className="min-h-0">{current?.body}</div>
    </aside>
  );
}
