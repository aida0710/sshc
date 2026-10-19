import type { DragEvent, MouseEvent, PointerEvent } from "react";
import type { HostEntry, HostMetadata } from "../api/config";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Icon } from "../ui/icons";
import { OperatingSystemIcon } from "../ui/OperatingSystemIcon";

// 「…」はマウスでは行に乗ったときだけ出し、タッチでは常に出す。
export const revealOnHover = "opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100 pointer-coarse:opacity-100";

function targetFor(host: HostEntry): string {
  if ((host.hostName ?? "") === "") return host.file.path ?? host.file.absolute;
  const account = (host.user ?? "") === "" ? "" : `${host.user}@`;
  const port = (host.port ?? "") === "" || host.port === "22" ? "" : `:${host.port}`;
  return `${account}${host.hostName}${port}`;
}

type ConnectionRowProps = {
  host: HostEntry;
  os: HostMetadata["os"];
  colour: string;
  tags: readonly string[];
  duplicateAlias: boolean;
  descriptionId: string;
  // opened はエディタで開いている行、chosen は複数選択に入っている行。
  // choosing のあいだ（1件でも選んでいるか、スマホの選択中）は chosen の行だけを強く見せる。
  opened: boolean;
  chosen: boolean;
  choosing: boolean;
  // スマホの選択中は、行の先頭にチェックを出す。
  showCheckbox: boolean;
  draggable: boolean;
  menuItems: readonly ActionMenuItem[];
  menuOpen: boolean;
  onMenuOpenChange: (open: boolean) => void;
  onClick: (event: MouseEvent) => void;
  onPointerDown: (event: PointerEvent) => void;
  onPointerEnd: () => void;
  onDragStart: (event: DragEvent) => void;
  onDragEnd: () => void;
};

// ConnectionRow は、接続先の一覧の1行（開く・選ぶボタンと、その接続先の「…」）である。
export function ConnectionRow({
  host, os, colour, tags, duplicateAlias, descriptionId,
  opened, chosen, choosing, showCheckbox, draggable,
  menuItems, menuOpen, onMenuOpenChange,
  onClick, onPointerDown, onPointerEnd, onDragStart, onDragEnd,
}: ConnectionRowProps) {
  const t = useTranslate();
  const highlighted = choosing ? chosen : opened;
  return (
    <li
      className="group/row relative border-b border-hairline last:border-b-0"
      onContextMenu={menuItems.length === 0 ? undefined : (event) => {
        // タッチの長押しは選択に使うので、メニューは開かない。
        if ((event.nativeEvent as globalThis.PointerEvent).pointerType === "touch") return;
        event.preventDefault();
        onMenuOpenChange(true);
      }}
    >
      <button
        type="button"
        aria-label={host.identity.alias}
        title={host.identity.alias}
        aria-current={opened ? "true" : undefined}
        aria-pressed={choosing ? chosen : undefined}
        aria-describedby={descriptionId}
        draggable={draggable}
        onMouseDown={(event) => {
          // Shift を押したクリックで、ブラウザが文字を範囲選択しないようにする。
          if (event.shiftKey) event.preventDefault();
        }}
        onClick={onClick}
        onPointerDown={onPointerDown}
        onPointerUp={onPointerEnd}
        onPointerLeave={onPointerEnd}
        onPointerCancel={onPointerEnd}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        className={`min-h-14 w-full select-none border-l-[3px] px-3 py-2.5 pr-11 text-left text-sm transition-colors ${highlighted ? "border-accent bg-select-fill text-ink" : "border-transparent text-ink-muted hover:bg-hover hover:text-ink"}`}
      >
        <span className="flex min-w-0 items-center gap-2">
          {showCheckbox ? (
            <span
              aria-hidden="true"
              className={`flex size-5 shrink-0 items-center justify-center rounded border ${chosen ? "border-accent bg-accent text-card" : "border-control-line bg-control"}`}
            >
              {chosen ? <Icon name="check" className="size-3.5" /> : null}
            </span>
          ) : null}
          <OperatingSystemIcon os={os} colour={colour} compact />
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className={`truncate ${highlighted ? "font-semibold" : "font-medium"}`}>{host.identity.alias}</span>
              {duplicateAlias ? <Icon name="warning" className="size-3.5 text-notice-ink" /> : null}
            </span>
            <span className={`mt-0.5 block truncate font-mono text-xs leading-4 ${highlighted ? "text-ink-muted" : "text-ink-faint"}`}>
              {targetFor(host)}
              {/* 同じ名前の接続先が並ぶときは、どのファイルの行かを添えて見分けられるようにする。 */}
              {duplicateAlias ? ` · ${host.file.path ?? host.file.absolute}:${host.line}` : null}
            </span>
            {tags.length === 0 ? null : (
              <span aria-hidden="true" className="mt-1 flex flex-wrap gap-1">
                {tags.map((tag) => <span key={tag} className="rounded bg-toolbar px-1 text-[0.65rem] text-ink-muted">{tag}</span>)}
              </span>
            )}
          </span>
        </span>
      </button>
      {menuItems.length === 0 ? null : (
        <div className={`absolute right-2 top-1/2 -translate-y-1/2 ${menuOpen ? "opacity-100" : revealOnHover}`}>
          <ActionMenu
            label={t("tree.connectionActions", { alias: host.identity.alias })}
            items={menuItems}
            open={menuOpen}
            onOpenChange={onMenuOpenChange}
            triggerClassName="flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-card hover:text-ink"
          />
        </div>
      )}
      <span id={descriptionId} className="sr-only">
        {[duplicateAlias ? t("tree.duplicateAlias") : "", targetFor(host), host.file.path ?? host.file.absolute].filter(Boolean).join(", ")}
      </span>
    </li>
  );
}
