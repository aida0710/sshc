// 生成物である。手で編集しない。
//
// 出どころは internal/validate/shortcut.go で、配っているのは cmd/rulegen である。
// 変えるならあちらを変えて make generate を走らせること。
//
// 和音は "Ctrl+Alt+Shift+Meta+キー" の形で、修飾はこの順に 0 個以上並ぶ。
// ファンクションキー以外のキーには Ctrl・Alt・Meta のどれかが要る。

export const shortcutKeyPattern = /^([A-Z0-9]|F([1-9]|1[0-9]|2[0-4])|Arrow(Up|Down|Left|Right)|PageUp|PageDown|Home|End|Insert|Delete|Backspace|Enter|Tab|Escape|Space|[-=,.;/\[\]\\'`])$/;
export const shortcutFunctionKeyPattern = /^F([1-9]|1[0-9]|2[0-4])$/;

export const shortcutModifiers: readonly string[] = ["Ctrl", "Alt", "Shift", "Meta"];
export const shortcutCommandModifiers: readonly string[] = ["Ctrl", "Alt", "Meta"];

// ショートカットを割り当てられる操作。並びは設定画面の並びでもある。
export const shortcutActions = ["palette", "terminalSearch", "copy", "paste", "nextSession", "previousSession", "home", "sftp"] as const;

export const maxShortcutPresets = 64;
export const maxShortcutPresetNameLength = 80;
export const maxShortcutKeysPerAction = 3;
