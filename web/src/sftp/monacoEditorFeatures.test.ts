import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const sourceDirectory = dirname(fileURLToPath(import.meta.url));
// Where "monaco-editor/<path>.js" points, by the package's exports map.
const monacoModules = join(sourceDirectory, "..", "..", "node_modules", "monaco-editor", "esm", "vs");

// The modules `file` imports statically, as absolute paths. A dynamic import()
// is left out: its module is not evaluated with `file`. An import cannot
// contain a parenthesis or a semicolon before its path, which keeps the match
// from running on into a later import().
function staticImports(file: string): string[] {
  const source = readFileSync(file, "utf8");
  return [...source.matchAll(/^\s*(?:import|export)\s[^'"();]*?['"](\.{1,2}\/[^'"]+)['"]/gm)]
    .map(([, path]) => join(dirname(file), path ?? ""));
}

// Every module that evaluating `entries` evaluates.
function evaluatedModules(entries: string[]): Set<string> {
  const evaluated = new Set<string>();
  const pending = [...entries];
  for (let file = pending.pop(); file !== undefined; file = pending.pop()) {
    if (evaluated.has(file)) continue;
    evaluated.add(file);
    if (file.endsWith(".js")) pending.push(...staticImports(file));
  }
  return evaluated;
}

// The Monaco modules monacoEditorFeatures.ts imports.
function editorFeatureModules(): string[] {
  const source = readFileSync(join(sourceDirectory, "monacoEditorFeatures.ts"), "utf8");
  return [...source.matchAll(/^import "monaco-editor\/([^"]+)";$/gm)].map(([, path]) => join(monacoModules, path ?? ""));
}

describe("monacoEditorFeatures", () => {
  it("registers every module the JSON support loads with the editor, so opening a JSON file registers no editor feature late", () => {
    // MonacoEditor.tsx imports editor.api.js before the features.
    const registered = evaluatedModules([join(monacoModules, "editor", "editor.api.js"), ...editorFeatureModules()]);
    // The worker client of the JSON support, which imports the editor features.
    const jsonSupportClient = join(monacoModules, "internal", "common", "workers.js");
    expect(staticImports(join(monacoModules, "languages", "features", "json", "workerManager.js"))).toContain(jsonSupportClient);

    const loadedLate = staticImports(jsonSupportClient).filter((module) => !registered.has(module));

    expect(editorFeatureModules().length).toBeGreaterThan(0);
    expect(loadedLate).toEqual([]);
  });
});
