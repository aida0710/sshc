// Load this module only from what loadMonacoEditor.ts loads: it imports Monaco.
import * as monaco from "monaco-editor/editor/editor.api.js";
import { sshConfigKeywords } from "./sshConfigKeywords";

// sshConfigLanguageId is Monaco's name for the language of ssh_config(5), which Monaco does not have.
export const sshConfigLanguageId = "ssh-config";

// The colours: a line's first word is its keyword (Host and Match start a block), the rest are its values. OpenSSH
// reads # as a comment only at the start of a line. Tokens (%h) and environment variables (${HOME}) stand for values
// filled in as ssh connects.
const sshConfigTokens: monaco.languages.IMonarchLanguage = {
  ignoreCase: true,
  tokenizer: {
    root: [
      [/^\s*#.*$/, "comment"],
      [/^(\s*)(host|match)\b/, ["white", "keyword.control"]],
      [/^(\s*)([a-z][a-z0-9]*)/, ["white", "keyword"]],
      [/"[^"]*"?/, "string"],
      [/%[a-z%]/, "variable"],
      [/\$\{[a-z0-9_]+\}/, "variable"],
      [/\b(?:yes|no|ask|confirm|none|auto|any|force)\b/, "type"],
      [/\b\d+[smhdw]?\b/, "number"],
      [/[=,]/, "delimiter"],
      [/\s+/, "white"],
      [/[^\s"%$=,]+/, ""],
    ],
  },
};

// keywordAt is the part of a line's first word typed so far, when the cursor is still in it.
function keywordAt(line: string): string | null {
  const match = /^\s*([A-Za-z0-9]*)$/.exec(line);
  return match === null ? null : match[1] ?? "";
}

let registered = false;

// registerSSHConfigLanguage teaches Monaco ssh_config: its colours, and the keywords offered (Ctrl+Space) while the
// first word of a line is typed. Once is enough for the page.
export function registerSSHConfigLanguage(): void {
  if (registered) return;
  registered = true;
  monaco.languages.register({ id: sshConfigLanguageId });
  monaco.languages.setMonarchTokensProvider(sshConfigLanguageId, sshConfigTokens);
  monaco.languages.setLanguageConfiguration(sshConfigLanguageId, { comments: { lineComment: "#" } });
  monaco.languages.registerCompletionItemProvider(sshConfigLanguageId, {
    provideCompletionItems(model, position) {
      const typed = keywordAt(model.getLineContent(position.lineNumber).slice(0, position.column - 1));
      if (typed === null) return { suggestions: [] };
      const range = new monaco.Range(position.lineNumber, position.column - typed.length, position.lineNumber, position.column);
      return {
        suggestions: sshConfigKeywords.map((keyword) => ({
          label: keyword,
          kind: monaco.languages.CompletionItemKind.Keyword,
          insertText: `${keyword} `,
          range,
        })),
      };
    },
  });
}
