// Monaco Editor's mode in which Tab moves focus, which Monaco keeps for the
// whole page. monaco-editor ships no types for this module, and its public API
// has no way to read the mode: editor.getOption(EditorOption.tabFocusMode)
// returns only the editor's own tabFocusMode option. Declares the part
// MonacoEditor.tsx reads (monaco-editor 0.56.0).
declare module "monaco-editor/editor/browser/config/tabFocus.js" {
  export const TabFocus: {
    getTabFocusMode(): boolean;
  };
}
