import type { Translate } from "../i18n/context";
import { shortcutActions, type Bindings, type ShortcutAction } from "../keyconfig/bindings";
import { shortcutActionLabels } from "../keyconfig/shortcutActionLabels";
import { terminalActionNames, type TerminalAction } from "../terminal/terminalActions";
import type { PaletteCommand } from "./CommandPalette";

// The palette does not list the action that opens it, and lists SFTP among the commands it leads with.
type ListedAction = Exclude<ShortcutAction, "palette" | "sftp">;

// Words a person may type for an action, in either language whichever one the screen is in.
const actionKeywords: Record<ListedAction, string> = {
  terminalSearch: "find search terminal output 検索 ターミナル",
  copy: "copy selection clipboard コピー 選択",
  paste: "paste clipboard 貼り付け ペースト",
  previousPrompt: "previous prompt shell 前 プロンプト",
  nextPrompt: "next prompt shell 次 プロンプト",
  increaseFontSize: "font text size zoom in larger 文字 フォント 大きさ 拡大",
  decreaseFontSize: "font text size zoom out smaller 文字 フォント 大きさ 縮小",
  resetFontSize: "font text size zoom reset 文字 フォント 大きさ 戻す",
  quickCommands: "quick commands snippets クイックコマンド",
  newSession: "new session terminal open 新しい セッション ターミナル",
  closeSession: "close session exit 閉じる セッション",
  nextSession: "next session connection switch 次 セッション 切り替え",
  previousSession: "previous session connection switch 前 セッション 切り替え",
  nextPane: "next pane split 次 ペイン 分割",
  previousPane: "previous pane split 前 ペイン 分割",
  splitRight: "split right pane 右 分割 ペイン 並べる",
  splitBelow: "split below down pane 下 分割 ペイン 並べる",
  toggleNavigation: "navigation sidebar menu toggle hide show ナビゲーション サイドバー メニュー 開閉",
  home: "home ホーム",
};

function isTerminalAction(action: ShortcutAction): action is TerminalAction {
  return (terminalActionNames as readonly string[]).includes(action);
}

function isListed(action: ShortcutAction): action is ListedAction {
  return action !== "palette" && action !== "sftp";
}

// paletteActionCommands are the shortcut actions that can act now, in the order of the settings page, each with
// the keys assigned to it. An action of a terminal needs a session to act on.
export function paletteActionCommands({ t, bindings, appActions, hasSession, runOnTerminal }: {
  t: Translate;
  bindings: Bindings;
  appActions: Partial<Record<ShortcutAction, () => void>>;
  hasSession: boolean;
  runOnTerminal: (action: TerminalAction) => void;
}): PaletteCommand[] {
  return shortcutActions.filter(isListed).flatMap((action) => {
    const run = isTerminalAction(action) ? (hasSession ? () => runOnTerminal(action) : undefined) : appActions[action];
    if (run === undefined) return [];
    return [{ id: `action:${action}`, label: t(shortcutActionLabels[action]), detail: "", search: actionKeywords[action], keys: bindings[action], run }];
  });
}
