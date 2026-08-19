import type { MouseEvent } from "react";
import { ConsoleList } from "../terminal/ConsoleList";
import { UpdateBadge } from "./UpdateBadge";
import { Icon, type IconName } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sectionPath, type Section } from "../routing/sectionRoute";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { TerminalSession } from "../api/integrations";

// AppNavigation は、窓の左の柱である。
//
// **下半分に 2 つの面がある。** 設定の一覧と、開いているコンソールの一覧である。
// Start はその上に固定されるので、どちらの面を見ていても出口はある。以前は
// App の 320 行の render の中に、帯と本文と一緒に並んでいた。
export function AppNavigation({
  navigationId,
  navigationOpen,
  navGroups,
  section,
  sectionIcons,
  sectionLabels,
  onNavigate,
  currentFace,
  onFaceChange,
  consoles,
  orderedConsoles,
  activeConsole,
  onShowConsole,
  onDuplicateConsole,
  onReorderConsoles,
  onOpenShell,
}: {
  navigationId: string;
  navigationOpen: boolean;
  navGroups: { label: MessageKey; sections: Section[] }[];
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
  currentFace: NavFace;
  onFaceChange: (face: NavFace) => void;
  consoles: TerminalSessionsState;
  orderedConsoles: TerminalSession[];
  activeConsole: string | null;
  onShowConsole: (id: string) => void;
  onDuplicateConsole: (id: string) => void;
  onReorderConsoles: (order: string[]) => void;
  onOpenShell: () => void;
}) {
  const t = useTranslate();
  const navigationLink = (name: Section) => (
    <NavigationLink
      name={name}
      section={section}
      sectionIcons={sectionIcons}
      sectionLabels={sectionLabels}
      onNavigate={onNavigate}
    />
  );
  return (
  <nav
    id={navigationId}
    aria-label={t("shell.primaryNavigation")}
    // 狭い画面ではこの格子の外に出て、面の上に重なる。md 以上では列へ
    // 戻り、translate も z も打ち消されるので、開閉の state は無視される。
    className={`fixed inset-y-0 left-0 z-30 flex w-72 min-h-0 flex-col overflow-hidden border-r border-line bg-sidebar p-2 transition-transform md:static md:z-auto md:w-auto md:translate-x-0 ${
      navigationOpen ? "translate-x-0" : "-translate-x-full"
    }`}
  >
    <div className="shrink-0">
    {/*
      Start はトグルより上に固定する。ターミナルの面を見ているときでも
      出口がひとつも無い状態を作らないためである。
    */}
    {navGroups.slice(0, 1).map((group) => (
      <div key={group.label} className="mb-2">
        <span aria-hidden="true" className="block px-2 pt-2 pb-1 text-xs font-semibold text-ink-muted">
          {t(group.label)}
        </span>
        <ul aria-label={t(group.label)}>
          {group.sections.map((name) => (
            <li key={name}>{navigationLink(name)}</li>
          ))}
        </ul>
      </div>
    ))}
    </div>
    <div
      role="tablist"
      aria-label={t("shell.navFaces")}
      className="my-2 grid shrink-0 grid-flow-col rounded-lg border border-control-line bg-control p-0.5"
    >
      {(["settings", "terminal"] as NavFace[]).map((face) => (
        <button
          key={face}
          type="button"
          role="tab"
          aria-selected={face === currentFace}
          onClick={() => onFaceChange(face)}
          className={`rounded-md px-2 py-1 text-xs ${
            face === currentFace ? "bg-card text-ink shadow-sm" : "text-ink-muted"
          }`}
        >
          {t(face === "settings" ? "shell.navFaceSettings" : "shell.navFaceTerminal")}
        </button>
      ))}
    </div>
    {/* 溢れるのはここだけである。開いているコンソールは何本にもなる。 */}
    <div className="min-h-0 flex-1 overflow-y-auto">
    {currentFace === "terminal" ? (
      <ConsoleList
        sessions={orderedConsoles}
        selected={activeConsole}
        maxSessions={consoles.maxSessions}
        busy={consoles.busy}
        problem={consoles.problem}
        onSelect={onShowConsole}
        onClose={(id) => void consoles.close(id)}
        onRename={(id, title) => consoles.rename(id, title)}
        onDuplicate={onDuplicateConsole}
        onReorder={onReorderConsoles}
        onOpenShell={onOpenShell}
      />
    ) : (
      navGroups.slice(1).map((group) => (
        <div key={group.label} className="mb-2">
          <span aria-hidden="true" className="block px-2 pt-2 pb-1 text-xs font-semibold text-ink-muted">
            {t(group.label)}
          </span>
          <ul aria-label={t(group.label)}>
            {group.sections.map((name) => (
              <li key={name}>{navigationLink(name)}</li>
            ))}
          </ul>
        </div>
      ))
    )}
    </div>
    {/*
      バージョンはナビゲーションの最下部に置く。めったに見ない
      ものが置かれる場所であり、それを変える唯一のコントロールと共に。
      **これも動かない。** 溢れるのは上の一覧だけである。
    */}
    <div className="shrink-0">
      <UpdateBadge />
    </div>
  </nav>
  );
}

// ナビゲーションの下半分が見せる 2 つの面。
export type NavFace = "settings" | "terminal";

function NavigationLink({
  name,
  section,
  sectionIcons,
  sectionLabels,
  onNavigate,
}: {
  name: Section;
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
}) {
  const t = useTranslate();
  return (
    <a
      href={sectionPath(name)}
      aria-current={section === name ? "page" : undefined}
      onClick={(event) => {
        onNavigate(event, name);
      }}
      // 触れる画面でだけ広げる。**md 以上では元の密度に戻す** —— この一覧は
      // 1280x720 でスクロールせずに収まることを e2e が保証しており、全幅で
      // 高くすると末尾の History がビューポートから落ちる。指のための 44px は
      // 指のある画面のものである。
      className={`flex w-full items-center gap-2 rounded-md px-3 py-2.5 text-left text-sm md:px-2 md:py-1.5 ${
        section === name ? "bg-select-fill text-ink" : "text-ink hover:bg-select-fill"
      }`}
    >
      <Icon name={sectionIcons[name]} className="h-4 w-4 text-ink-muted" />
      {t(sectionLabels[name])}
    </a>
  );
}
