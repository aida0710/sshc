import { useEffect, useRef } from "react";
import * as monaco from "monaco-editor/editor/editor.api.js";
import { languageFor } from "./MonacoEditor";
import { useTheme } from "../theme/context";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";

export function MonacoDiffEditor({ path, original, modified }: { path: string; original: string; modified: string }) {
  const container = useRef<HTMLDivElement>(null);
  const { resolved } = useTheme();
  useEffect(() => {
    if (container.current === null) return;
    const originalModel = monaco.editor.createModel(original, languageFor(path));
    const modifiedModel = monaco.editor.createModel(modified, languageFor(path));
    const editor = monaco.editor.createDiffEditor(container.current, {
      readOnly: true, originalEditable: false, automaticLayout: true,
      renderSideBySide: true, useInlineViewWhenSpaceIsLimited: true,
      // This comparison has no apply/revert actions or their gutter menu.
      renderGutterMenu: false,
      minimap: { enabled: false }, scrollBeyondLastLine: false,
      fontFamily: "JetBrains Mono, ui-monospace, monospace", fontSize: 13,
      theme: resolved === "dark" ? "vs-dark" : "vs", unusualLineTerminators: "off",
    });
    editor.setModel({ original: originalModel, modified: modifiedModel });
    return () => {
      // Detach the models while the diff editor's context and observers are live.
      editor.setModel(null);
      editor.dispose();
      originalModel.dispose();
      modifiedModel.dispose();
    };
    // Theme updates do not discard the comparison's scroll position.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, original, modified]);
  useEffect(() => { monaco.editor.setTheme(resolved === "dark" ? "vs-dark" : "vs"); }, [resolved]);
  return <div ref={container} {...keyboardOwnerProps} className="h-full min-h-64 w-full overflow-hidden" />;
}
