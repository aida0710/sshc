import { InspectorToggle, type InspectorContent } from "../ui/Inspector";
import { Icon } from "../ui/icons";
import { autoControl } from "../ui/form";
import { useLanguage } from "../i18n/context";
import { locales, type Locale } from "../i18n/locale";
import { themes, type Theme } from "../theme/theme";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";

// AppHeader は、窓のいちばん上の帯である。
//
// **ここに置いてあるのは、どの画面でも同じものだけである。** ナビゲーションの
// 開閉、いまどこを見ているか、engine が動いているか、そして見た目と言語。
// 以前は App の 320 行の render の中に、ナビゲーションと本文と一緒に並んでいた。
export function AppHeader({
  route,
  version,
  state,
  navigationOpen,
  navigationId,
  onToggleNavigation,
  inspector,
  inspectorOpen,
  onToggleInspector,
  sectionLabels,
  themeLabels,
  localeLabels,
  theme,
  onThemeChange,
}: {
  route: { kind: string; section?: Section };
  version: string;
  state: string;
  navigationOpen: boolean;
  navigationId: string;
  onToggleNavigation: () => void;
  inspector: InspectorContent | null;
  inspectorOpen: boolean;
  onToggleInspector: () => void;
  sectionLabels: Record<Section, MessageKey>;
  themeLabels: Record<Theme, MessageKey>;
  localeLabels: Record<Locale, MessageKey>;
  theme: Theme;
  onThemeChange: (theme: Theme) => void;
}) {
  const { t, locale, setLocale } = useLanguage();
  return (
  <header className="relative z-20 flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2.5 md:gap-3 md:px-6">
    <button
      type="button"
      // ランドマークと同じ名前にしない。**開いているかどうかは名前では
      // なく aria-expanded が運ぶ** —— Inspector のトグルと同じ作法である。
      aria-label={t("shell.navigationToggle")}
      aria-expanded={navigationOpen}
      aria-controls={navigationId}
      onClick={() => onToggleNavigation()}
      className="shrink-0 rounded-md border border-control-line bg-card p-2 md:hidden"
    >
      <Icon name="menu" className="h-4 w-4" />
    </button>
    {/*
      アプリケーション名は引き続き h1 であり、開いているセクションは
      見出しにせずその横に表示する。セクションを見出しにしてしまうと
      "Known Hosts" と "Remote Keys" が見出し名前空間に二重に入る——
      ここで一回、パネルでもう一回——Playwright はアクセシブル
      ネームを部分一致で照合するため、スイートのページレベルクエリは
      それらの見出しを二つ見つけてしまい、失敗する。
    */}
    <h1 className="hidden shrink-0 whitespace-nowrap text-xs font-medium text-ink-muted md:block">{t("shell.title")}</h1>
    <span aria-hidden="true" className="hidden text-xs text-ink-faint md:inline">/</span>
    <p className="shrink-0 whitespace-nowrap text-sm font-semibold">
      {route.kind === "section" && route.section !== undefined
        ? t(sectionLabels[route.section])
        : t("shell.pageNotFound")}
    </p>
    {/*
      狭い画面で落とすのは**文字だけ**である。要素ごと hidden にすると
      アクセシビリティツリーからも消え、状態を目で読めない人には稼働の
      有無が伝わらなくなる。残るドットが、見る人のための同じ報せである。
    */}
    <p role="status" className="flex min-w-0 items-center gap-1.5 truncate text-xs text-ink-muted">
      <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full bg-live" />
      <span className="sr-only sm:not-sr-only sm:truncate">
        {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
      </span>
    </p>
    {inspector === null ? (
      <span className="ml-auto" />
    ) : (
      <span className="ml-auto">
        <InspectorToggle
          label={inspector.label}
          open={inspectorOpen}
          attention={inspector.attention}
          onToggle={() => onToggleInspector()}
        />
      </span>
    )}
    <label htmlFor="appearance" className="hidden shrink-0 whitespace-nowrap text-sm text-ink-muted md:inline">
      {t("shell.theme")}
    </label>
    {/*
      狭い画面では細くする。**360px にこの 2 つを丸ごと並べる幅は無く**、
      言語の select が画面の外へはみ出していた。中の文字は詰められるが、
      開けば選択肢は全部読める——2 か所に置いて名前を曖昧にするより、
      1 か所を狭めるほうが安い。

      **min-w-0 が要る。** max-w は「これ以上広げない」であって「これ以下に
      縮めてよい」ではない。flex 項目の既定の min-width:auto は select を
      その min-content——いちばん長い選択肢の幅——より細くさせないので、
      文字が広く出るフォントの機械では、上限を付けていても行が溢れる。
      実際 CI がそうで、言語の select の右端だけが 3px 外に出ていた。
      どれだけ溢れるかはフォント次第なので、その幅を削るのではなく、
      **縮んでよいと言う。**
    */}
    <select
      id="appearance"
      value={theme}
      onChange={(event) => onThemeChange(event.target.value as Theme)}
      className={`${autoControl} min-w-0 max-w-24 md:max-w-none`}
    >
      {themes.map((candidate) => (
        <option key={candidate} value={candidate}>
          {t(themeLabels[candidate])}
        </option>
      ))}
    </select>
    <label htmlFor="language" className="hidden shrink-0 whitespace-nowrap text-sm text-ink-muted md:inline">
      {t("shell.language")}
    </label>
    <select
      id="language"
      value={locale}
      onChange={(event) => setLocale(event.target.value as Locale)}
      className={`${autoControl} min-w-0 max-w-24 md:max-w-none`}
    >
      {locales.map((candidate) => (
        <option key={candidate} value={candidate}>
          {t(localeLabels[candidate])}
        </option>
      ))}
    </select>
  </header>
  );
}
