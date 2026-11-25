import type { ReactNode } from "react";
import { Icon } from "./icons";
import { menuChoiceItemClass, menuGroupClass, menuGroupHeadingClass, menuItemClass } from "./menuStyle";
import { SwitchMark } from "./SwitchMark";

// MenuItem は、選ぶと何かをするメニューの項目。閉じるのは呼び手が onSelect の中で決める。
export function MenuItem({ tone, disabled = false, title, onSelect, children }: {
  tone?: "danger" | undefined;
  disabled?: boolean;
  // title は、押せない項目がなぜ押せないか、または項目の補足。
  title?: string | undefined;
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" role="menuitem" disabled={disabled} title={title} onClick={onSelect} className={menuItemClass(tone)}>
      {children}
    </button>
  );
}

// MenuChoiceItem は、入れる・切る（menuitemcheckbox、右にスイッチ）か、いくつかから1つを選ぶ（menuitemradio、
// 選んだものに印）項目。indent は、上の項目に属する選択肢を一段下げる。hint は項目の補足（title）。
export function MenuChoiceItem({ role, label, hint, checked, onSelect, indent = false }: {
  role: "menuitemcheckbox" | "menuitemradio";
  label: string;
  hint: string;
  checked: boolean;
  onSelect: () => void;
  indent?: boolean;
}) {
  return (
    <button type="button" role={role} aria-checked={checked} className={menuChoiceItemClass({ indent })} title={hint} onClick={onSelect}>
      <span>{label}</span>
      {role === "menuitemcheckbox" ? <SwitchMark on={checked} />
        : checked ? <Icon name="check" className="size-4 text-live" /> : <span aria-hidden="true" className="size-4" />}
    </button>
  );
}

// MenuGroup は、上の項目と線で分け、見出しを付けた項目のまとまり。見出しはまとまりの名前として読み上げる。
export function MenuGroup({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={heading} className={menuGroupClass}>
      <p aria-hidden="true" className={menuGroupHeadingClass}>{heading}</p>
      {children}
    </div>
  );
}
