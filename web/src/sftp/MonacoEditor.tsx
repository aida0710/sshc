import { useEffect, useRef } from "react";
import EditorWorker from "monaco-editor/editor/editor.worker.js?worker";
import JSONWorker from "monaco-editor/language/json/json.worker.js?worker";
import * as monaco from "monaco-editor/editor/editor.api.js";
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

type MonacoEditorProps = {
  path: string;
  value: string;
  onChange: (value: string) => void;
  readOnly?: boolean;
};

type MonacoHost = typeof globalThis & {
  MonacoEnvironment?: { getWorker: (_moduleId: string, label: string) => Worker };
};

const host = globalThis as MonacoHost;
host.MonacoEnvironment ??= {
  // The worker is emitted as a same-origin Vite asset. Avoiding Monaco's blob
  // fallback lets the application keep its existing strict script policy.
  getWorker: (_moduleId, label) => label === "json" ? new JSONWorker() : new EditorWorker(),
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

export function MonacoEditor({ path, value, onChange, readOnly = false }: MonacoEditorProps) {
  const container = useRef<HTMLDivElement>(null);
  const callback = useRef(onChange);
  const current = useRef(value);
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const { resolved } = useTheme();
  callback.current = onChange;
  current.current = value;

  useEffect(() => {
    if (container.current === null) return;
    const model = monaco.editor.createModel(value, languageFor(path));
    const view = monaco.editor.create(container.current, {
      model,
      readOnly,
      automaticLayout: true,
      minimap: { enabled: false },
      fontFamily: "JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      fontSize: 13,
      lineNumbersMinChars: 3,
      padding: { top: 10, bottom: 10 },
      scrollBeyondLastLine: false,
      theme: resolved === "dark" ? "vs-dark" : "vs",
    });
    editor.current = view;
    const subscription = model.onDidChangeContent(() => {
      const next = model.getValue();
      current.current = next;
      callback.current(next);
    });
    return () => {
      subscription.dispose();
      view.dispose();
      model.dispose();
      editor.current = null;
    };
    // A changed path deliberately creates a new model. The parent owns dirty
    // navigation confirmation before it changes this key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  useEffect(() => {
    const model = editor.current?.getModel();
    if (model !== null && model !== undefined && model.getValue() !== value) {
      model.setValue(value);
    }
  }, [value]);

  useEffect(() => {
    editor.current?.updateOptions({
      readOnly,
      theme: resolved === "dark" ? "vs-dark" : "vs",
    });
  }, [readOnly, resolved]);

  return <div ref={container} className="h-full min-h-64 w-full overflow-hidden" />;
}

export { languageFor };
