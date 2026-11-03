// 生成物である。手で編集しない。
//
// 出どころは internal/validate/keybar.go で、配っているのは cmd/rulegen である。
// 変えるならあちらを変えて make generate を走らせること。

// キーバーに置ける用意したキーの名前。
export const keyBarKeys = ["DirectInput", "Control", "Alt", "Escape", "Tab", "Shift+Tab", "Enter", "Backspace", "Delete", "Insert", "ArrowLeft", "ArrowUp", "ArrowDown", "ArrowRight", "Home", "End", "PageUp", "PageDown", "Ctrl+C", "Ctrl+D", "Ctrl+Z", "Ctrl+L", "Ctrl+R", "Ctrl+A", "Ctrl+E", "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12"] as const;

// 自作のキーに出す文字と、送る内容に使えない文字。
export const keyBarLabelForbiddenPattern = /[\x00-\x1f\x7f-\x9f]/u;
export const keyBarTextForbiddenPattern = /[\x00-\x08\x0b-\x1f\x7f-\x9f]/u;

export const maxKeyBarPresets = 64;
export const maxKeyBarPresetNameLength = 80;
export const maxKeyBarKeys = 16;
export const maxKeyBarMoreKeys = 48;
export const maxKeyBarLabelLength = 8;
export const maxKeyBarTextLength = 1000;
