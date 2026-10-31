// Registers Monaco's editor features: Find and Replace, Go to Line, the context
// menu, the command palette (F1), comments, folding, bracket matching, multiple
// cursors and the rest. Each is a module that registers itself when it is
// evaluated, and editor.api.js registers none of them.
//
// Import this before anything creates a Monaco model or editor. Monaco takes
// the services the features need only once, the first time it needs any of its
// services: at the latest when it creates its first model or editor. A feature
// registered after that still reaches the editors created later, but they
// cannot start it when the service it needs was not taken: the page gets
// "[createInstance] ... depends on UNKNOWN service ICodeLensCache".
//
// Every feature is registered, not a chosen few, for two reasons:
// - The JSON support loads every feature anyway: its worker client
//   (monaco-editor/internal/common/workers.js) imports them. With a chosen few
//   here, the rest would arrive with the first JSON file, too late for their
//   services.
// - Their code is shared with the JSON support, so registering every feature
//   adds almost nothing to the embedded UI. The code sits in a chunk the editor
//   loads when it is first opened.
//
// features/register.all.js is Monaco's entry point for every editor feature
// (monaco-editor 0.56.0 CHANGELOG). The four imports after it are modules that
// Monaco's full editor (editor.main.js) and the JSON support load but
// register.all.js leaves out. With them, the editor is the same before and
// after a JSON file is opened, as monacoEditorFeatures.test.ts checks.
import "monaco-editor/features/register.all.js";
import "monaco-editor/editor/contrib/caretOperations/browser/caretOperations.js";
import "monaco-editor/editor/contrib/dropOrPasteInto/browser/copyPasteContribution.js";
import "monaco-editor/editor/contrib/gotoError/browser/markerSelectionStatus.js";
import "monaco-editor/editor/contrib/semanticTokens/browser/documentSemanticTokens.js";
