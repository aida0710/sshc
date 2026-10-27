import type { SnippetVariable } from "./api";

// placeholders は、コマンドに書かれた {{name}} の変数名を、最初に出てきた順に1回ずつ返す。
export function placeholders(command: string): string[] {
  return [
    ...new Set(
      [...command.matchAll(/\{\{([A-Za-z_][A-Za-z0-9_]{0,63})\}\}/g)]
        .map((match) => match[1] ?? "")
        .filter(Boolean),
    ),
  ];
}

// variablesFor は、コマンドにある変数の定義を返す。選んであった型は残し、コマンドから消えた
// 変数は落とし、新しい変数は必須の文字列にする。
export function variablesFor(
  command: string,
  previous: SnippetVariable[],
): SnippetVariable[] {
  const byName = new Map(previous.map((variable) => [variable.name, variable]));
  return placeholders(command).map(
    (name) => byName.get(name) ?? { name, type: "string", required: true },
  );
}
