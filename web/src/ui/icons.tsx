import {
  Activity,
  Bell,
  Check,
  ChevronRight,
  Clock,
  CloudUpload,
  Copy,
  Ellipsis,
  FileCog,
  Folder,
  GripVertical,
  House,
  KeyRound,
  LockKeyhole,
  Maximize,
  Menu,
  PanelRight,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Server,
  Settings,
  ShieldCheck,
  SquareTerminal,
  X,
  type LucideIcon,
} from "lucide-react";

// 画面のアイコンは lucide（https://lucide.dev/icons/）から選び、形を手で描かない。
// 手で描いた形は、似た別のもの（歯車のつもりが太陽）に見えることがあった。
// 名前は画面の中での役割で付け、どの lucide のアイコンを使うかはここだけで決める。
const shapes = {
  home: House,
  connections: Server,
  config: FileCog,
  groups: Folder,
  keys: KeyRound,
  knownHosts: ShieldCheck,
  remoteKeys: CloudUpload,
  diagnostics: Activity,
  secrets: LockKeyhole,
  settings: Settings,
  notification: Bell,
  sync: RefreshCw,
  history: Clock,
  inspector: PanelRight,
  moreHorizontal: Ellipsis,
  terminal: SquareTerminal,
  movePane: GripVertical,
  focus: Maximize,
  close: X,
  plus: Plus,
  menu: Menu,
  search: Search,
  chevronRight: ChevronRight,
  edit: Pencil,
  copy: Copy,
  check: Check,
} satisfies Record<string, LucideIcon>;

export type IconName = keyof typeof shapes;

export const iconNames = Object.keys(shapes) as IconName[];

// iconStrokeWidth は、手で描いていたころのアイコンと同じ線の太さである。lucide の既定（2）
// にすると、画面のアイコンがすべて一段太くなる。
const iconStrokeWidth = 1.7;

export function Icon({ name, className = "h-4 w-4" }: { name: IconName; className?: string }) {
  const Shape = shapes[name];
  return <Shape aria-hidden="true" data-icon={name} className={`shrink-0 ${className}`} strokeWidth={iconStrokeWidth} />;
}
