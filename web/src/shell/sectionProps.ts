import type { CreateConnectionDraft, CreationPrerequisite } from "../connections/CreateConnectionModal";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "../keys/workflow";
import type { BrowserLocation, NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import type { FileTarget } from "../explorer/ConfigExplorer";
import type { Section } from "../routing/sectionRoute";
import type { InspectorContent } from "../ui/Inspector";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { TerminalSettings } from "../api/integrations";

// ここにあるのは、外殻がセクションへ渡す値を関心ごとに束ねたものである。
//
// **以前は 23 個の prop が平らに並んでいた。** どれが一緒に動くのかは、App.tsx を
// 上から下まで読まないと分からず、画面をひとつ足すたびに束が太った。葉の
// コンポーネントが受け取る形は変えていない——変えたのは、外殻の中を通る束だけである。

// Navigation は、いまどこに居て、どこへ行けるか。
export type Navigation = {
  location: BrowserLocation;
  fileTarget: FileTarget | null;
  onNavigate: (section: Section) => void;
  onNavigateLocation: (url: string, options?: NavigateLocationOptions) => void;
  onNavigateForCreation: (section: CreationPrerequisite) => void;
  onOpenFile: (path: string, line: number) => void;
  // onNavigationBlockerChange は、保存していない編集がある画面が「いま離れると
  // 失われる」と申告するための口である。**申告するのは画面の側である** ——
  // 外殻は何が未保存かを知らない。
  onNavigationBlockerChange: (blocker: NavigationBlocker | null) => void;
};

// Handoff は、画面をまたいで受け渡される途中の仕事である。
//
// **鍵を作った画面と、それを使う画面は別である。** 生成した直後に「この鍵を
// どの接続へ」と尋ねられる経路があり、その答えは接続の画面にしかない。作りかけの
// 接続も同じで、グループや鍵を先に用意しに行って戻ってくる。
export type Handoff = {
  connectionKey: GeneratedPrivateKeyHandoff | null;
  publicKey: GeneratedPublicKeyHandoff | null;
  connectionDraft: CreateConnectionDraft | null;
  onAssignGeneratedKey: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey: (key: GeneratedPublicKeyHandoff) => void;
  onConnectionKeyApplied: () => void;
  onPublicKeyHandled: () => void;
  onConnectionDraftChange: (draft: CreateConnectionDraft | null) => void;
};

// Shell は、外殻そのものが持つ面である。錠、調査ペイン、コンソール、端末の設定。
export type Shell = {
  onLock: () => void;
  onInspector: (content: InspectorContent) => void;
  consoles: TerminalSessionsState;
  onShowConsole: (id: string) => void;
  onTerminalSettingsChange: (settings: TerminalSettings) => void;
};

// Declared は、設定が宣言しているものである。**推測しない** ——ディレクトリが
// グループなのは ~/.ssh/config の一行が宣言するからで、読むのは configuration API
// だけである。
export type Declared = {
  groups: string[];
  knownAliases: string[];
};
