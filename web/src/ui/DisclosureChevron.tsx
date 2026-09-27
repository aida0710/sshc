import { Icon } from "./icons";

// detailsTurn は、この印が入っている <summary> の <details> が開いているときに下を向かせる。
// 入れ子の <details> で外側だけが開いているときに内側の印まで回らないよう、group-open ではなく
// 「開いた details の直下の summary の中」に限る。
const detailsTurn = "[details[open]>summary_&]:rotate-90";

// DisclosureChevron は、開閉できるボタンや見出しに添える印である。閉じているときは右を、
// 開いているときは下を向く。expanded を渡さないときは、<summary> の中に置き、その <details>
// が開いているかで向きを変える。
export function DisclosureChevron({ expanded, className = "size-4" }: { expanded?: boolean; className?: string }) {
  const turn = expanded === undefined ? detailsTurn : expanded ? "rotate-90" : "";
  return <Icon name="chevronRight" className={`transition-transform ${turn} ${className}`} />;
}
