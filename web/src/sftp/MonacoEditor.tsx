import { useTranslate } from "../i18n/context";
import { useEffect, useRef } from "react";
// Load this module only through loadMonacoEditor (loadMonacoEditor.ts). Monaco
// creates its Trusted Types policies through MonacoEnvironment while the
// imports below are evaluated, so the environment must be installed first.
import * as monaco from "monaco-editor/editor/editor.api.js";
import { TabFocus } from "monaco-editor/editor/browser/config/tabFocus.js";
// Find and Replace and the other editor features. They must be registered
// before the first model or editor is created (monacoEditorFeatures.ts).
import "./monacoEditorFeatures";
import "monaco-editor/languages/definitions/css/register.js";
import "monaco-editor/languages/definitions/go/register.js";
import "monaco-editor/languages/definitions/html/register.js";
import "monaco-editor/languages/definitions/javascript/register.js";
import "monaco-editor/language/json/monaco.contribution.js";
import "monaco-editor/languages/definitions/markdown/register.js";
import "monaco-editor/languages/definitions/python/register.js";
import "monaco-editor/languages/definitions/shell/register.js";
import "monaco-editor/languages/definitions/typescript/register.js";
import "monaco-editor/languages/definitions/yaml/register.js";
import { useTheme } from "../theme/context";
import { editorCursorBlinking, reducedMotionQuery } from "../ui/reducedMotion";
import { useMediaQuery } from "../ui/useMediaQuery";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";

type MonacoEditorProps = {
  path: string;
  value: string;
  onChange: (value: string) => void;
  readOnly?: boolean;
  initialLine?: number;
  onSave?: (() => void) | undefined;
};

function languageFor(path: string): string {
  const name = path.toLowerCase();
  if (name.endsWith(".json") || name.endsWith(".jsonc")) return "json";
  if (name.endsWith(".yaml") || name.endsWith(".yml")) return "yaml";
  if (name.endsWith(".js") || name.endsWith(".mjs") || name.endsWith(".cjs")) return "javascript";
  if (name.endsWith(".ts") || name.endsWith(".tsx")) return "typescript";
  if (name.endsWith(".sh") || name.endsWith(".bash") || name.endsWith(".zsh")) return "shell";
  if (name.endsWith(".md") || name.endsWith(".markdown")) return "markdown";
  if (name.endsWith(".go")) return "go";
  if (name.endsWith(".py")) return "python";
  if (name.endsWith(".html") || name.endsWith(".htm")) return "html";
  if (name.endsWith(".css")) return "css";
  return "plaintext";
}

// Holds back the editor features (monacoEditorFeatures.ts) that would change
// the remote file without the user asking for it:
// - Suggestions show only on Ctrl+Space, not while typing. A suggestion shown
//   while typing takes the Enter or Tab meant for a new line or an indent, and
//   replaces the typed word with another word of the file.
// - A file with unusual line terminators, such as Line Separator (U+2028), is
//   opened as it is. Monaco would otherwise ask in a browser dialog to remove
//   them. The dialog names the file by Monaco's model number and points at a
//   setting sshc does not have, and removing them changes the file.
const noUnrequestedEditOptions = {
  quickSuggestions: false,
  suggestOnTriggerCharacters: false,
  unusualLineTerminators: "off",
} satisfies monaco.editor.IStandaloneEditorConstructionOptions;

// Monaco's command for the mode in which Tab moves focus instead of
// indenting: Ctrl+M (Ctrl+Shift+M on macOS), or "Toggle Tab Key Moves Focus"
// in the editor's command palette.
const toggleTabMovesFocusCommand = "editor.action.toggleTabFocusMode";

// Monaco keeps the mode in which Tab moves focus for the whole page
// (monacoTabFocus.d.ts), and the editor shows nothing while the mode is on.
// Each editor starts with Tab indenting, so that the mode switched on in one
// file does not keep Tab from indenting in the files opened after it. The mode
// is switched off with Monaco's own command, which also announces to a screen
// reader that Tab now inserts the tab character.
function startWithTabIndenting(view: monaco.editor.IStandaloneCodeEditor): void {
  if (!TabFocus.getTabFocusMode()) return;
  view.trigger("sshc", toggleTabMovesFocusCommand, null);
}

export function MonacoEditor({ path, value, onChange, readOnly = false, initialLine = 1, onSave }: MonacoEditorProps) {
  const t = useTranslate();
  const container = useRef<HTMLDivElement>(null);
  const callback = useRef(onChange);
  const saveCallback = useRef(onSave);
  // The contents the editor reported through onChange that have not come back
  // as `value` yet, oldest first. React can render one of them after the user
  // has typed more, so `value` is not always the model's latest contents.
  const pendingReports = useRef<string[]>([]);
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const { resolved } = useTheme();
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  callback.current = onChange;
  saveCallback.current = onSave;

  useEffect(() => {
    if (container.current === null) return;
    const model = monaco.editor.createModel(value, languageFor(path));
    const view = monaco.editor.create(container.current, {
      model,
      readOnly,
      ...noUnrequestedEditOptions,
      automaticLayout: true,
      minimap: { enabled: false },
      fontFamily: "JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      fontSize: 13,
      lineNumbersMinChars: 3,
      padding: { top: 10, bottom: 10 },
      scrollBeyondLastLine: false,
      cursorBlinking: editorCursorBlinking(reducedMotion),
      theme: resolved === "dark" ? "vs-dark" : "vs",
    });
    startWithTabIndenting(view);
    const saveAction = view.addAction({ id: "sshc.saveFile", label: t("sftp.save"), keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS], run: () => { saveCallback.current?.(); } });
    editor.current = view;
    pendingReports.current = [];
    const subscription = model.onDidChangeContent(() => {
      const next = model.getValue();
      pendingReports.current.push(next);
      callback.current(next);
    });
    return () => {
      saveAction.dispose();
      subscription.dispose();
      view.dispose();
      model.dispose();
      editor.current = null;
    };
    // A changed path deliberately creates a new model. The parent owns dirty
    // navigation confirmation before it changes this key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  // Writes a `value` the parent set itself, such as the contents read again
  // from the remote, into the model. A `value` the editor reported is skipped
  // even when it differs from the model: it is an older report rendered after
  // the user typed more, and writing it would undo that typing and move the
  // cursor to the start of the file.
  useEffect(() => {
    const model = editor.current?.getModel();
    if (model === null || model === undefined) return;
    const report = pendingReports.current.indexOf(value);
    if (report !== -1) {
      pendingReports.current.splice(0, report + 1);
      return;
    }
    if (model.getValue() !== value) model.setValue(value);
    // Also drops the report of the setValue above: the parent set that value.
    pendingReports.current = [];
  }, [value]);

  useEffect(() => {
    editor.current?.updateOptions({
      readOnly,
      cursorBlinking: editorCursorBlinking(reducedMotion),
      theme: resolved === "dark" ? "vs-dark" : "vs",
    });
  }, [readOnly, reducedMotion, resolved]);

  useEffect(() => {
    const view = editor.current;
    const lineCount = view?.getModel()?.getLineCount();
    if (view === null || lineCount === undefined) return;
    const line = Math.max(1, Math.min(initialLine, lineCount));
    view.setPosition({ lineNumber: line, column: 1 });
    view.revealLineInCenter(line);
  }, [path, initialLine]);

  return <div ref={container} {...keyboardOwnerProps} className="h-full min-h-64 w-full overflow-hidden" />;
}

export { languageFor };
