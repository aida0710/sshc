import { Suspense, lazy, useCallback, useEffect, useMemo, useState, type MouseEvent } from "react";
import { apiClient, whenLocked, type HealthResponse } from "./api/client";
import { integrationsApi, type PasswordVaultStatus, type TerminalAppearance, type TerminalSettings } from "./api/integrations";
import { resolveAppearance } from "./terminal/appearance";
import { configApi } from "./api/config";
import type { SessionState } from "./session/bootstrap";
import type { CreateConnectionDraft, CreationPrerequisite } from "./connections/CreateConnectionModal";
import type { FileTarget } from "./explorer/ConfigExplorer";
import { LockScreen } from "./secrets/LockScreen";
import { OverviewPanel } from "./overview/OverviewPanel";
import { useLanguage, useTranslate } from "./i18n/context";
import type { Locale } from "./i18n/locale";
import { secondaryAction } from "./ui/form";
import { IconSprite, type IconName } from "./ui/icons";
import { InspectorPane, type InspectorContent } from "./ui/Inspector";
import { useTheme } from "./theme/context";
import type { Theme } from "./theme/theme";
import type { MessageKey } from "./i18n/messages";
import { Button } from "./ui/surface";
import { sectionPath, type Section } from "./routing/sectionRoute";
import { AppHeader } from "./shell/AppHeader";
import { AppNavigation, type NavFace } from "./shell/AppNavigation";
import type { Declared, Handoff, Navigation, Shell } from "./shell/sectionProps";
import {
  useSectionRoute,
} from "./routing/useSectionRoute";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "./keys/workflow";
import { useTerminalSessions, type TerminalSessionsState } from "./terminal/sessions";

// xterm.js は 400 kB を超える。どの画面の chunk に入れても、端末を開かない人が
// その重さを払うことになる。だから端末だけを別の chunk に切り、コンソールを
// 選んだときに初めて読む。
//
// これは体感の話にとどまらない。接続画面は URL の正規化をマウント後に行うので、
// chunk が重くなるとそのリダイレクトも遅れる。end-to-end はそれを捉えた。
const TerminalView = lazy(() =>
  import("./terminal/TerminalView").then(({ TerminalView }) => ({ default: TerminalView })),
);
const ConnectionsPage = lazy(() =>
  import("./connections/ConnectionsPage").then(({ ConnectionsPage }) => ({ default: ConnectionsPage })),
);
const ConfigExplorer = lazy(() =>
  import("./explorer/ConfigExplorer").then(({ ConfigExplorer }) => ({ default: ConfigExplorer })),
);
const GroupsPanel = lazy(() =>
  import("./groups/GroupsPanel").then(({ GroupsPanel }) => ({ default: GroupsPanel })),
);
const HistoryPanel = lazy(() =>
  import("./history/HistoryPanel").then(({ HistoryPanel }) => ({ default: HistoryPanel })),
);
const KeysScreen = lazy(() =>
  import("./keys/KeysScreen").then(({ KeysScreen }) => ({ default: KeysScreen })),
);
const DiagnosticsPanel = lazy(() =>
  import("./diagnostics/DiagnosticsPanel").then(({ DiagnosticsPanel }) => ({ default: DiagnosticsPanel })),
);
const SecretsPanel = lazy(() =>
  import("./secrets/SecretsPanel").then(({ SecretsPanel }) => ({ default: SecretsPanel })),
);
const SettingsPanel = lazy(() =>
  import("./settings/SettingsPanel").then(({ SettingsPanel }) => ({ default: SettingsPanel })),
);
const SyncPanel = lazy(() =>
  import("./sync/SyncPanel").then(({ SyncPanel }) => ({ default: SyncPanel })),
);
const KnownHostsPanel = lazy(() =>
  import("./knownhosts/KnownHostsPanel").then(({ KnownHostsPanel }) => ({ default: KnownHostsPanel })),
);
const RemoteKeyPanel = lazy(() =>
  import("./remotekeys/RemoteKeyPanel").then(({ RemoteKeyPanel }) => ({ default: RemoteKeyPanel })),
);

type AppProps = {
  bootstrap: () => Promise<SessionState>;
  health: () => Promise<HealthResponse>;
  // vault はアプリケーションが開いているかどうかを答える。bootstrap や health と
  // 同じ理由でここに注入されている——テストが動かすのはシェル自身の状態機械で
  // あって、その下のトランスポートではないからだ。
  vault?: () => Promise<PasswordVaultStatus>;
};

// セクション識別子は英語のまま訳さない。これはこのコンポーネント自身の
// ルーティング語彙であり、訳してしまうとどのパネルが開いているかが表示
// 言語に依存することになってしまう。
const sectionLabels: Record<Section, MessageKey> = {
  Home: "section.home",
  Connections: "section.connections",
  Terminal: "section.terminal",
  Config: "section.config",
  Groups: "section.groups",
  Keys: "section.keys",
  "Known Hosts": "section.knownHosts",
  "Remote Keys": "section.remoteKeys",
  Diagnostics: "section.diagnostics",
  Secrets: "section.secrets",
  Settings: "section.settings",
  Sync: "section.sync",
  History: "section.history",
};

const localeLabels: Record<Locale, MessageKey> = {
  en: "shell.languageEnglish",
  ja: "shell.languageJapanese",
};

const sectionIcons: Record<Section, IconName> = {
  Home: "home",
  Connections: "connections",
  Terminal: "terminal",
  Config: "config",
  Groups: "groups",
  Keys: "keys",
  "Known Hosts": "knownHosts",
  "Remote Keys": "remoteKeys",
  Diagnostics: "diagnostics",
  Secrets: "secrets",
  Settings: "settings",
  Sync: "sync",
  History: "history",
};

// 十個のセクションを平たく並べただけでは、どれとどれが近いのか手がかりがない。
//
// グループラベルはリストに付けた `aria-label` と、目のための `aria-hidden`
// span である——意図的に見出しにはしていない。Playwright はアクセシブル
// ネームを部分一致で照合するため、見出しを "Keys and hosts" にすると、end-to-end
// スイートの "Keys" へのページレベルクエリが二重に一致し、strict モードで失敗する。
// 先頭のグループはトグルより上に固定される。Home と Connections と Terminal が
// そこにあるのは、ターミナルの面から行を選ぶとその画面へ連れて行かれるからで、
// 戻る道も、行った先も、同じ高さに無いと釣り合わない。
const navGroups: { label: MessageKey; sections: Section[] }[] = [
  { label: "shell.navStart", sections: ["Home", "Connections", "Terminal"] },
  { label: "shell.navConnections", sections: ["Config", "Groups"] },
  { label: "shell.navKeysHosts", sections: ["Keys", "Known Hosts", "Remote Keys"] },
  { label: "shell.navMaintenance", sections: ["Diagnostics", "Secrets", "Settings", "Sync", "History"] },
];


const themeLabels: Record<Theme, MessageKey> = {
  system: "shell.themeSystem",
  light: "shell.themeLight",
  dark: "shell.themeDark",
};

// ハンバーガーが指す先。aria-controls は id を要求する。
const navigationId = "primary-navigation";

export function App({ bootstrap, health, vault = integrationsApi.passwordVault }: AppProps) {
  const { t } = useLanguage();
  const { theme, setTheme } = useTheme();
  const { route, location, navigate, navigateLocation, setNavigationBlocker } = useSectionRoute();
  const section = route.kind === "section" ? route.section : null;
  const terminalFace = section === "Terminal";
  // "locked" はアプリケーション全体を指し、その中の一画面ではない。あらゆる
  // 書き込みはマスターパスワードで封じたバックアップを残すため、vault が
  // 閉じたまま使える状態というものは存在しない。
  const [state, setState] = useState<"starting" | "locked" | "ready" | "error">("starting");
  // 失敗の名前。**画面に出す。** 「開始できませんでした」だけを読んだ人に
  // できることは何も無く、devtools を開けない機械では他に知る手段が無い。
  // ここに入るのは bootstrap.ts が投げる固定の識別子か、fetch の型名だけで、
  // 入口の fragment はそのどちらにも現れない。
  const [failure, setFailure] = useState("");
  const [vaultExists, setVaultExists] = useState(false);
  // 預けてあるかどうかは、解錠の画面が開いた瞬間に要る——**自分からプロンプトを
  // 出すため**であり、押されてから調べるのでは間に合わない。
  const [vaultBiometric, setVaultBiometric] = useState(false);
  const [version, setVersion] = useState("");
  const [fileTarget, setFileTarget] = useState<FileTarget | null>(null);
  // 宣言済みのグループ名は、セッションが立ち上がった時点で一度だけ読む。
  // Keys 画面は移動先としてそれらを提示するだけで、ディレクトリからグルー
  // プを推測しない。ディレクトリがグループになるのはエントリファイルが宣言する場合だけだ。
  const [groups, setGroups] = useState<string[]>([]);
  // 接続ごとに選ばれた見た目。別名で引く。
  const [hostAppearance, setHostAppearance] = useState<Map<string, TerminalAppearance>>(new Map());
  const [knownAliases, setKnownAliases] = useState<string[]>([]);
  // Only non-secret connection fields may outlive the creation modal. This
  // draft lets a person create a group or key and return without starting the
  // form again; passwords remain local to the modal and are cleared on exit.
  const [connectionDraft, setConnectionDraft] = useState<CreateConnectionDraft | null>(null);
  // 鍵生成後の次アクションだけを、ページをまたぐ短命な状態として持つ。
  // ID と ~/.ssh からの相対パスだけで、パスフレーズや鍵本文はここへ来ない。
  const [preferredConnectionKey, setPreferredConnectionKey] = useState<GeneratedPrivateKeyHandoff | null>(null);
  const [preferredPublicKey, setPreferredPublicKey] = useState<GeneratedPublicKeyHandoff | null>(null);
  const consumePreferredConnectionKey = useCallback(() => setPreferredConnectionKey(null), []);
  const consumePreferredPublicKey = useCallback(() => setPreferredPublicKey(null), []);
  // ペインはシェルに属するものであり、セクションに属するものではない。
  // Connections で開いたものは Keys でも開いたままになる——セクションを
  // 切り替えるたびに自分で閉じるペインでは、頻繁に開き直す羽目になる。
  // これはホストについての好みではなく、ウィンドウについての好みだからだ。
  // ナビゲーションのドロワーは狭い画面にだけ存在する。**これは媒体クエリ
  // ではない** —— md 以上では Tailwind 側がこの state を無視する形にして
  // あるので、幅が変わったときに畳み直す処理も要らない。
  const [navigationOpen, setNavigationOpen] = useState(false);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspector, setInspector] = useState<InspectorContent>(null);

  // Esc でドロワーを閉じる。**nav の onKeyDown ではなく document に付ける** ——
  // 開いた直後にフォーカスがドロワーの中にあるとは限らず、そのときに閉じられ
  // なければ、行き止まりを作ったことになる。
  useEffect(() => {
    if (!navigationOpen) return;
    function close(event: KeyboardEvent) {
      if (event.key === "Escape") setNavigationOpen(false);
    }
    document.addEventListener("keydown", close);
    return () => document.removeEventListener("keydown", close);
  }, [navigationOpen]);
  // 開いているセッションはセクションに属さない。どの画面を見ていても同じ
  // ものが開いているので、一覧はナビゲーションと同じ高さ——シェル——が持つ。
  // URL には載せない。共有可能な URL に載せる価値のある状態ではない。
  const consoles = useTerminalSessions(integrationsApi, t, state === "ready");
  const [terminalSettings, setTerminalSettings] = useState<TerminalSettings>({});
  const [activeConsole, setActiveConsole] = useState<string | null>(null);
  const [navFace, setNavFace] = useState<NavFace | null>(null);

  // 端末は設定画面を開かなくても設定を使う。読めないときは安全に既定へ倒し、
  // copy/paste 自体を理由にアプリケーションを開けなくすることはしない。
  useEffect(() => {
    if (state !== "ready") return;
    let active = true;
    void integrationsApi.terminalSettings()
      .then((settings) => {
        if (active) setTerminalSettings(settings);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [state, section]);

  // 既定はターミナル側だが、セッションが 1 本も無ければ設定側から始める。
  //
  // セッションはこのプロセスの寿命までしか生きないので、起動直後は必ず 0 本で
  // ある。無条件にターミナル側を既定にすると、毎回の起動が空の一覧から始まり、
  // 他の画面へ行くにはトグルを探すことになる。決めるのは最初の一覧が届いた
  // 一度だけで、そのあとは人が選んだ面のままにする。
  useEffect(() => {
    if (navFace !== null || !consoles.loaded) return;
    setNavFace(consoles.sessions.length > 0 ? "terminal" : "settings");
  }, [consoles.loaded, consoles.sessions.length, navFace]);

  // 閉じられたセッションを主画面に残さない。
  useEffect(() => {
    if (activeConsole === null) return;
    if (!consoles.sessions.some((session) => session.id === activeConsole)) setActiveConsole(null);
  }, [consoles.sessions, activeConsole]);

  // 開いているものがあるなら、どれかを選んでおく。
  //
  // **選ばれていない状態は、リロードのあとに必ず起きる。** セッションは常駐
  // プロセス側で生きているが、どれを見ていたかはこのプロセスの記憶であり、
  // 読み込み直せば消える。そのとき Terminal の画面は「開いているコンソールが
  // ありません」と言っていた——**一覧に何本も並んでいる隣で。**
  //
  // 閉じた直後にも起きる。上の効果が選択を外すので、そこで次の 1 本へ移る。
  useEffect(() => {
    if (activeConsole !== null || consoles.sessions.length === 0) return;
    setActiveConsole(consoles.sessions[0]?.id ?? null);
  }, [consoles.sessions, activeConsole]);

  // 端末が描かれるのは Terminal である。ナビゲーションはどの画面からでも
  // 押せるので、選んだらそこへ連れて行く。
  const showConsole = useCallback(
    (id: string) => {
      setActiveConsole(id);
      // **ドロワーも畳む。** コンソールの一覧はナビゲーションの中に居るので、
      // 狭い画面でそこから開くと、開いた端末をドロワー自身が覆う。セクション
      // のリンクと同じ理由であり、広い画面ではドロワーが無いので何も起きない。
      setNavigationOpen(false);
      navigate("Terminal");
      // Home から開かれたセッションは、この写しにまだ載っていない。
      void consoles.refresh();
    },
    [navigate, consoles],
  );

  // 並び順はこの画面のものであって、セッションの性質ではない。サーバーは
  // 開いた順に返し、人が動かした分だけをここが覚える。metadata へ書かないのは
  // セッションがこのプロセスの寿命までしか生きないからで、書けば孤児が残る。
  const [consoleOrder, setConsoleOrder] = useState<string[]>([]);
  const orderedConsoles = useMemo(() => {
    const rank = new Map(consoleOrder.map((id, index) => [id, index]));
    // 並べ替えたことのないセッションは、サーバーが返した順のまま後ろに続く。
    return consoles.sessions
      .map((session, index) => ({ session, rank: rank.get(session.id) ?? consoleOrder.length + index }))
      .sort((left, right) => left.rank - right.rank)
      .map((entry) => entry.session);
  }, [consoles.sessions, consoleOrder]);

  // **この 2 つを渡す先は void を期待している。** 渡すときは `void` を綴る
  // （下の AppNavigation を見よ）。どちらも中で consoles.open を呼び、あれは
  // 自分で catch して null を返すので、ここへ落ちてくる失敗は無い。それでも
  // 黙って渡さないのは、後で open が投げるようになった日に、**理由がどこにも
  // 出ないまま画面だけが動かなくなる**からである。
  const openLocalShell = useCallback(async () => {
    const opened = await consoles.open({ kind: "shell" });
    if (opened !== null) showConsole(opened.id);
  }, [consoles, showConsole]);

  // 複製は、同じ相手へもう一本つなぐことである。設定ファイルには触れない——接続
  // そのものの複製は Connections 画面にある別の操作である。
  const duplicateConsole = useCallback(
    async (id: string) => {
      const session = consoles.sessions.find((candidate) => candidate.id === id);
      if (session === undefined) return;
      const opened = await consoles.open(
        session.kind === "ssh" && session.alias !== undefined
          ? { kind: "ssh", alias: session.alias }
          : { kind: "shell" },
      );
      if (opened !== null) showConsole(opened.id);
    },
    [consoles, showConsole],
  );

  // ペインの中身はどのセクションが開いているかに属するが、開閉状態自体は
  // そうではない。したがってセクションを離れるとペインの中身は消去され
  // るが、ペイン自体が閉じられることはない。
  //
  // ヘッダーにはセクションが埋められるスロットはもう無い。かつては
  // Connections ツリーの並び替えコントロール用のスロットがあり、ここ
  // でクリアする実装のせいで、他の画面に移った瞬間にそのコントロールが消えてしまっていた。
  // 一つのリストに属するコントロールは、今はそのリストの中にある。
  useEffect(() => {
    setInspector(null);
  }, [section]);

  // セクション切り替えはシェルが所有するので、ブロックをファイルと行でし
  // か指し示せないビューは、自分でルーティングせずジャンプをここに委ねる。
  function openFile(path: string, line: number) {
    setFileTarget({ path, line });
    navigate("Config");
  }

  function assignGeneratedKey(key: GeneratedPrivateKeyHandoff) {
    setPreferredConnectionKey(key);
    navigate("Connections");
  }

  function installGeneratedKey(key: GeneratedPublicKeyHandoff) {
    setPreferredPublicKey(key);
    navigate("Remote Keys");
  }

  function followSectionLink(event: MouseEvent<HTMLAnchorElement>, target: Section) {
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    event.preventDefault();
    navigate(target);
  }

  // 面が決まるまでは設定側を描く。最初の一覧が届くのを待つあいだ、空の
  // ターミナル面を一瞬見せてから入れ替えるより、静かである。
  const currentFace: NavFace = navFace ?? "settings";


  useEffect(() => {
    let active = true;
    void bootstrap()
      .then((sessionState) => {
        if (!active) return null;
        apiClient.setCSRF(sessionState.csrfToken);
        return health();
      })
      .then((result) => {
        if (!active || result === null) return null;
        setVersion(result.version);
        return vault();
      })
      .then((status) => {
        if (!active || status === null) return;
        setVaultExists(status.exists);
        setVaultBiometric(status.biometric.enabled);
        if (!status.unlocked) {
          setState("locked");
          return;
        }
        setState("ready");
      })
      .catch((reason: unknown) => {
        console.error("sshc could not start its session", reason);
        if (!active) return;
        setFailure(reason instanceof Error ? reason.message : String(reason));
        setState("error");
      });

    return () => {
      active = false;
      apiClient.clear();
    };
  }, [bootstrap, health, vault]);

  // vault は一日使われないと自動的に閉じ、それはどの二つのリクエストの
  // 間にも起こり得る。クライアントはそれをここで一度だけ報告し、シェル
  // を入口へ戻す。そうしないと、もう使えなくなったアプリケーションに対
  // して、あらゆる画面が個別に失敗を報告することになってしまう。
  useEffect(() => {
    whenLocked(() => {
      setVaultExists(true);
      setState("locked");
    });
    return () => whenLocked(null);
  }, []);

  // 宣言済みグループは、面が変わるたびに読み直す。閉じている間は読まない
  // ——応答できるはずのルートも、それまでは拒否する。
  //
  // **一度きりでは足りなかった。** グループを作った直後に鍵の画面へ行くと、
  // 作ったばかりのフォルダがそこに無い——移動先の一覧はこの値から組まれて
  // いるので、再読込するまで新しいフォルダへは移せなかった。読み直すのは
  // 一度の GET であり、面を変えたときにしか起きない。
  useEffect(() => {
    if (state !== "ready") return;
    let active = true;
    void configApi
      .overview()
      .then((overview) => {
        if (!active) return;
        setGroups((overview.metadata.groups ?? []).map((group) => group.name));
        // 接続ごとに選ばれた見た目。**別名で引く。** 端末のセッションが名乗る
        // のは別名だけで、どのファイルの Host かは持っていない——OpenSSH 自身も
        // 先に見つけた方を使うので、ここも最初の一致を採る。
        setHostAppearance(
          new Map(
            (overview.metadata.hosts ?? []).flatMap((host) =>
              host.appearance === undefined || host.identity.alias === ""
                ? []
                : [[host.identity.alias, host.appearance] as const],
            ),
          ),
        );
        setKnownAliases([...new Set(overview.hosts.map((host) => host.identity.alias).filter((alias) => alias !== ""))]);
      })
      // グループを列挙できないシェルでも動作は成立する——移動先リストが
      // 空になるだけで、他のあらゆる画面には影響しない。
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [state, section]);

  if (state === "locked") {
    return (
      <LockScreen
        exists={vaultExists}
        biometric={vaultBiometric}
        onOpen={() => {
          // A successful initialise or unlock means the vault exists now. Keep
          // that fact when this same process locks again; otherwise a freshly
          // created vault is presented as an empty installation and the UI
          // offers an operation the server must reject as already existing.
          setVaultExists(true);
          setState("ready");
        }}
      />
    );
  }

  if (state === "error") {
    return (
      <main className="flex flex-col items-start gap-3 p-6">
        <h1 className="text-base font-semibold">{t("shell.title")}</h1>
        <p role="alert" className="text-sm text-danger">{t("shell.bootstrapFailed")}</p>
        {/*
          失敗の名前をそのまま出す。**翻訳しない** ——これは人に読ませる文では
          なく、報告に写して貼るための識別子である。訳せば、受け取った側は
          それがどの分岐かを言い当てられなくなる。
        */}
        {failure === "" ? null : (
          <code className="max-w-full overflow-x-auto rounded-md border border-line bg-card px-2 py-1 text-xs">
            {failure}
          </code>
        )}
        {/*
          もう一度だけ、入口の fragment 抜きで開き直す。**それがこの画面から
          出る唯一の道である** ——使い切られた fragment を持ったまま読み直せば
          何度でもここへ戻ってくるが、クッキーだけで届けば renew が答える。
        */}
        <Button
          kind="primary"
          onClick={() => window.location.replace(window.location.pathname + window.location.search)}
        >
          {t("shell.bootstrapRetry")}
        </Button>
      </main>
    );
  }

  // シェルの高さはちょうどビューポート一つ分で、全体としてスクロール
  // することはない。ヘッダーと主要なナビゲーションは固定され、main の
  // 中でパネルだけがスクロールする。ページ全体がスクロールしてしまうと
  // セッション状態とセクションボタンが画面外に流れてしまい、ローカル
  // セッションがまだ生きているかを報告するステータス行を持つシェルとしては、それは誤りだ。
  //
  // それを実現しているのが body 行の min-h-0 だ。flex の子要素の
  // min-height は既定でコンテンツサイズになるため、これが無いと丈の高い
  // パネルが行をビューポートより押し広げ、ドキュメントが再びスクロール
  // してしまう。grid-rows-[minmax(0,1fr)] は暗黙の行に対して同じ役割を
  // 果たす——無ければその行は与えられた枠に収まらず、コンテンツのサイズで決まってしまう。
  //
  // 二つのスクロール領域に付けた `relative` は装飾ではない。connection
  // tree が書き出すスクリーンリーダー用の説明は `sr-only` であり、これは
  // `position: absolute` を意味する。絶対配置の要素が祖先の overflow で
  // クリップされるのは、その祖先が containing block である場合に限られる。
  // static な main はそうではないため、それらの span は初期 containing
  // block を基準に解決され、fold のはるか下の static な位置に置かれて、
  // ドキュメントのスクロール領域を引き伸ばしていた——パネル自体は正しく
  // クリップされて見える一方で、ヘッダーはまた画面外へ流れてしまっていた。
  //
  // ナビゲーションは一つではなく三つのリストである。それぞれのグループ
  // ラベルは `aria-label` と `aria-hidden` span であり、見出しには決して
  // しない。Playwright はアクセシブルネームを部分一致で照合するため、見
  // 出しを "Keys and hosts" にすると、end-to-end スイートの見出しレベル
  // 2 の "Keys" へのページレベルクエリが二重に一致し、strict モード違反で失敗する。
  return (
    <div className="flex h-screen flex-col bg-canvas text-ink">
      <IconSprite />
      {/*
        z-20 は inspector のシート (z-10) より上、ナビのドロワー (z-30) より下で
        ある。狭い画面で inspector が面を覆っても、それを閉じるトグルは必ず
        この帯の上に残る。
      */}
      <AppHeader
        route={route}
        version={version}
        state={state}
        navigationOpen={navigationOpen}
        navigationId={navigationId}
        onToggleNavigation={() => setNavigationOpen((open) => !open)}
        inspector={inspector}
        inspectorOpen={inspectorOpen}
        onToggleInspector={() => setInspectorOpen((open) => !open)}
        sectionLabels={sectionLabels}
        themeLabels={themeLabels}
        localeLabels={localeLabels}
        theme={theme}
        onThemeChange={setTheme}
      />
      <div
        className={`grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[15rem_minmax(0,1fr)] ${
          // minmax(0,…) on the middle track for the same reason min-h-0 is on
          // the row: a bare 1fr is minmax(auto,1fr), so the column refuses to
          // shrink below its content and the panel runs out under the
          // inspector instead of narrowing to make room for it.
          //
          // **grid-cols-1 が既定である。** 狭い画面に置ける面は 1 つで、ナビは
          // ドロワーとしてこの格子の外へ出て、inspector はシートになる。列が
          // 増えるのは幅が増えたときだけだ。
          inspector !== null && inspectorOpen ? "lg:grid-cols-[15rem_minmax(0,1fr)_17rem]" : ""
        }`}
      >
        {/*
          ナビゲーション自身はスクロールしない。**上半分——Start と面のトグル
          ——は常に同じ位置に居る。** ここが動くと、行が増えたときに出口の
          位置が変わり、探し直すことになる。溢れるのは下半分だけであり、
          そこだけが自分でスクロールする。
        */}
        {navigationOpen ? (
          <div
            aria-hidden="true"
            onClick={() => setNavigationOpen(false)}
            className="fixed inset-0 z-20 bg-canvas/70 md:hidden"
          />
        ) : null}
        <AppNavigation
          navigationId={navigationId}
          navigationOpen={navigationOpen}
          navGroups={navGroups}
          section={section}
          sectionIcons={sectionIcons}
          sectionLabels={sectionLabels}
          onNavigate={(event, name) => {
            // **遷移したらドロワーを畳む。** 開いたままだと、選んだ先が
            // 自分の後ろに隠れる。広い画面ではドロワーが無いので何も起きない。
            setNavigationOpen(false);
            followSectionLink(event, name);
          }}
          currentFace={currentFace}
          onFaceChange={setNavFace}
          consoles={consoles}
          orderedConsoles={orderedConsoles}
          activeConsole={activeConsole}
          onShowConsole={showConsole}
          onDuplicateConsole={(id) => void duplicateConsole(id)}
          onReorderConsoles={setConsoleOrder}
          onOpenShell={() => void openLocalShell()}
        />
        {/*
          ここに padding は無い。ウィンドウの端から端まで埋めたいセクション
          ——それ自身が一つの面である Connections のリスト——は、padding
          の付いた箱の中ではそれができない。だから padding を適用するのは
          セクション側の役目であり、SectionView がそれを望む九画面に適用している。
        */}
        <main className="relative flex min-h-0 flex-col overflow-hidden">
          {connectionDraft !== null && (section === "Groups" || section === "Keys") ? (
            <div className="flex shrink-0 items-center gap-3 border-b border-notice-line bg-notice px-4 py-2 text-sm text-notice-ink">
              <p className="min-w-0 grow truncate">
                {t("conn.createDraftWaiting", { alias: connectionDraft.alias || t("conn.createUntitledDraft") })}
              </p>
              <Button className="shrink-0" onClick={() => navigate("Connections")}>
                {t("conn.createReturnToDraft")}
              </Button>
            </div>
          ) : null}
          {state === "ready" ? (
            <div className="relative min-h-0 flex-1 overflow-hidden">
              {/*
                **端末は面を離れても mount したままにする。** 外すと xterm ごと
                捨てることになり、戻ったときに読めるのはサーバー側のリング
                バッファの再生だけになる。あれは途中から始まるバイト列なので、
                alt-screen を使っているもの（vim、top）は崩れた姿で戻ってくる。
                隠すだけなら、画面も、選択も、スクロール位置も、そのまま残る。

                選ばれているコンソールが一本も無いうちは描かない。xterm は
                この束の中でいちばん重い塊であり、端末を開かない起動にそれを
                読み込ませる理由はない。
              */}
              {terminalFace || activeConsole !== null ? (
                <div className={terminalFace ? "h-full" : "hidden"}>
                  <TerminalScreen
                    consoles={consoles}
                    activeConsole={activeConsole}
                    settings={terminalSettings}
                    hostAppearance={hostAppearance}
                  />
                </div>
              ) : null}
              {route.kind === "section" ? (
                <Suspense fallback={null}>
                  <SectionView
                    section={route.section}
                    navigation={{
                      location,
                      fileTarget,
                      onNavigate: navigate,
                      onNavigateLocation: navigateLocation,
                      onNavigateForCreation: (target: CreationPrerequisite) => navigate(target),
                      onOpenFile: openFile,
                      onNavigationBlockerChange: setNavigationBlocker,
                    }}
                    handoff={{
                      connectionKey: preferredConnectionKey,
                      publicKey: preferredPublicKey,
                      connectionDraft,
                      onAssignGeneratedKey: assignGeneratedKey,
                      onInstallGeneratedKey: installGeneratedKey,
                      onConnectionKeyApplied: consumePreferredConnectionKey,
                      onPublicKeyHandled: consumePreferredPublicKey,
                      onConnectionDraftChange: setConnectionDraft,
                    }}
                    shell={{
                      onLock: () => setState("locked"),
                      onInspector: setInspector,
                      consoles,
                      onShowConsole: showConsole,
                      onTerminalSettingsChange: setTerminalSettings,
                    }}
                    declared={{ groups, knownAliases }}
                  />
                </Suspense>
              ) : (
                <div className="h-full overflow-y-auto p-6">
                  <section aria-labelledby="not-found-heading" className="flex max-w-2xl flex-col gap-3">
                    <h2 id="not-found-heading" className="font-medium">{t("shell.pageNotFound")}</h2>
                    <p className="text-sm text-ink-muted">{t("shell.pageNotFoundDescription")}</p>
                    <code className="w-fit rounded-md border border-line bg-card px-2 py-1 text-sm">
                      {route.pathname}
                    </code>
                    <a
                      href={sectionPath("Home")}
                      onClick={(event) => followSectionLink(event, "Home")}
                      className={`${secondaryAction} w-fit`}
                    >
                      {t("shell.goHome")}
                    </a>
                  </section>
                </div>
              )}
            </div>
          ) : null}
        </main>
        {inspector !== null && inspectorOpen ? (
          <InspectorPane label={inspector.label}>{inspector.body}</InspectorPane>
        ) : null}
      </div>
    </div>
  );
}

type SectionViewProps = {
  section: Section;
  navigation: Navigation;
  handoff: Handoff;
  shell: Shell;
  declared: Declared;
};

function SectionView(props: SectionViewProps) {
  // **端末は一画面である。** 接続の一覧と同じ画面に置くと、詳細を見る場所を
  // 端末が奪う——実際そうなっていた。ここは接続とは別の面であり、
  // 一覧の隣ではなく、一覧の代わりに出る。
  //
  // その面を描くのはここではない。**端末はこの木の外に住んでいる**——面を
  // 離れるたびに外されないためであり、ここに置けば、外すのはこの分岐になる。
  if (props.section === "Terminal") {
    return null;
  }
  // Connections は自前のペインをウィンドウの端まで配置する。それ以外の
  // セクションはすべて文書であり、文書には余白とスクロールバーが要る。
  if (props.section === "Connections") {
    return (
      <ConnectionsPage
        onOpenFile={props.navigation.onOpenFile}
        onInspector={props.shell.onInspector}
        creationDraft={props.handoff.connectionDraft}
        onCreationDraftChange={props.handoff.onConnectionDraftChange}
        onNavigateForCreation={props.navigation.onNavigateForCreation}
        location={props.navigation.location}
        onNavigateLocation={props.navigation.onNavigateLocation}
        onNavigationBlockerChange={props.navigation.onNavigationBlockerChange}
        preferredKey={props.handoff.connectionKey}
        onPreferredKeyApplied={props.handoff.onConnectionKeyApplied}
        consoles={props.shell.consoles}
        onShowConsole={props.shell.onShowConsole}
      />
    );
  }
  return <div className="h-full overflow-y-auto p-6">{<PaddedSection {...props} />}</div>;
}

// TerminalScreen は、開いているコンソールひとつを画面いっぱいに出す。
//
// **余白もスクロールも端末が自分で持つ**ので、この画面は素の箱でよい。
// 選ばれているものが無い、あるいは選ばれたものがもう無いときは、
// どこから開くのかを言う——空の黒い箱を見せない。
function TerminalScreen({
  consoles,
  activeConsole,
  settings,
  hostAppearance,
}: {
  consoles: TerminalSessionsState;
  activeConsole: string | null;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
}) {
  const t = useTranslate();
  const session = consoles.sessions.find((entry) => entry.id === activeConsole);
  // **接続が勝ち、全体はその下に敷く。** ローカルシェルは別名を持たないので、
  // そこでは常に全体の選択になる。
  const appearance = resolveAppearance(
    session?.alias === undefined ? undefined : hostAppearance.get(session.alias),
    settings.appearance,
  );
  if (session === undefined) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <section className="max-w-sm text-center" role="status">
          <h2 className="text-lg font-semibold text-ink">{t("terminal.emptyHeading")}</h2>
          <p className="mt-1 text-sm leading-6 text-ink-muted">{t("terminal.emptyHint")}</p>
        </section>
      </div>
    );
  }
  // **h-full が要る。** 高さの指定が無い flex の列は内容の高さまで伸びるので、
  // 端末は親を突き抜け、はみ出した分は overflow-hidden に切られて見えなく
  // なる——実際 1561px の端末が 666px の枠に入っていた。見えないだけでなく、
  // 向こうのシェルはその行数を信じるので、全画面を使うプログラムが画面外へ描く。
  return (
    <div className="flex h-full min-h-0 flex-col">
      <Suspense fallback={null}>
        {/*
          exactOptionalPropertyTypes の下では、undefined を渡すことと渡さない
          ことは別である。設定されていないなら渡さない——受け側の既定に任せる。
        */}
        <TerminalView
          key={session.id}
          session={session}
          {...(settings.fontSize === undefined ? {} : { fontSize: settings.fontSize })}
          {...(appearance.palette === "" ? {} : { palette: appearance.palette })}
          copyOnSelect={settings.copyOnSelect ?? true}
          rightClickPaste={settings.rightClickPaste ?? true}
          onExit={() => consoles.markExited(session.id)}
        />
      </Suspense>
    </div>
  );
}

function PaddedSection({ section, navigation, handoff, shell, declared }: SectionViewProps) {
  const { fileTarget, onNavigate, onNavigateLocation } = navigation;
  const { onLock, onInspector, consoles, onShowConsole, onTerminalSettingsChange } = shell;
  if (section === "Home") {
    return (
      <OverviewPanel
        onNavigate={onNavigate}
        onNavigateLocation={onNavigateLocation}
        onConsoleOpened={onShowConsole}
      />
    );
  }
  if (section === "Config") {
    return <ConfigExplorer target={fileTarget} />;
  }
  if (section === "Groups") {
    return <GroupsPanel onInspector={onInspector} />;
  }
  if (section === "Secrets") {
    return <SecretsPanel onLock={onLock} />;
  }
  if (section === "Settings") {
    return <SettingsPanel consoles={consoles} onTerminalSettingsChange={onTerminalSettingsChange} />;
  }
  if (section === "Sync") {
    return <SyncPanel />;
  }
  if (section === "History") {
    return <HistoryPanel />;
  }
  if (section === "Keys") {
    return (
      <KeysScreen
        onInspector={onInspector}
        groups={declared.groups}
        onAssignGeneratedKey={handoff.onAssignGeneratedKey}
        onInstallGeneratedKey={handoff.onInstallGeneratedKey}
      />
    );
  }
  if (section === "Known Hosts") {
    return <KnownHostsPanel />;
  }
  if (section === "Remote Keys") {
    return (
      <RemoteKeyPanel
        preferredPublicKeyPath={handoff.publicKey?.publicRelativePath ?? null}
        onPreferredPublicKeyHandled={handoff.onPublicKeyHandled}
      />
    );
  }
  if (section === "Diagnostics") {
    return <DiagnosticsPanel hosts={declared.knownAliases} />;
  }
  return (
    <section aria-labelledby="section-heading" className="flex flex-col gap-4">
      <h2 id="section-heading" className="font-medium">{section}</h2>
    </section>
  );
}
