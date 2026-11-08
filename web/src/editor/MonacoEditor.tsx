import { useTranslate } from "../i18n/context";
import { useEffect, useRef } from "react";
// Load this module only through loadMonacoEditor (loadMonacoEditor.ts). Monaco
// creates its Trusted Types policies through MonacoEnvironment while the
// imports below are evaluated, so the environment must be installed first.
import * as monaco from "monaco-editor/editor/editor.api.js";
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
import { editorFontFamily } from "./editorFont";
import { useTheme } from "../theme/context";
import { editorCursorBlinking } from "../ui/reducedMotion";
import { useReducedMotion } from "../theme/motionContext";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { noUnrequestedEditOptions, startWithTabIndenting } from "./monacoEditing";
import { useModelFollowsValue } from "./useModelFollowsValue";

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
  const { scheme } = useTheme();
  const reducedMotion = useReducedMotion();
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
      fontFamily: editorFontFamily,
      fontSize: 13,
      lineNumbersMinChars: 3,
      padding: { top: 10, bottom: 10 },
      scrollBeyondLastLine: false,
      cursorBlinking: editorCursorBlinking(reducedMotion),
      theme: scheme === "dark" ? "vs-dark" : "vs",
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
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  useModelFollowsValue(editor, value, pendingReports);

  useEffect(() => {
    editor.current?.updateOptions({
      readOnly,
      cursorBlinking: editorCursorBlinking(reducedMotion),
      theme: scheme === "dark" ? "vs-dark" : "vs",
    });
  }, [readOnly, reducedMotion, scheme]);

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
