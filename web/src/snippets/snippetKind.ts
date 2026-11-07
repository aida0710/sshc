import type { Translate } from "../i18n/context";
import type { Snippet, SnippetDraft, SnippetStep } from "./api";

// A snippet is one command, or steps that wait for a text in the shell's output before they type
// (internal/snippets/steps.go). Steps run only when a connection starts, so the places that type a snippet into a
// terminal now or run it on hosts offer the commands only.

// hasSteps reports whether a saved snippet is one of steps.
export function hasSteps(snippet: Pick<Snippet, "steps">): boolean {
  return (snippet.steps?.length ?? 0) > 0;
}

// commandSnippets are the snippets that are one command.
export function commandSnippets(snippets: readonly Snippet[]): Snippet[] {
  return snippets.filter((snippet) => !hasSteps(snippet));
}

// editsSteps reports whether a draft is being edited as steps; it is from the moment that kind is chosen, before any
// step has text.
export function editsSteps(draft: Pick<SnippetDraft, "steps">): boolean {
  return draft.steps !== undefined;
}

// templateText is the text whose {{placeholders}} the snippet's variables fill: its command, or what its steps send.
export function templateText(draft: Pick<SnippetDraft, "command" | "steps">): string {
  return editsSteps(draft) ? (draft.steps ?? []).map((step) => step.send).join("\n") : draft.command;
}

// snippetSummary is the line a list shows for a snippet: its command's first line, or how many steps it has.
export function snippetSummary(t: Translate, snippet: Pick<Snippet, "command" | "steps">): string {
  return hasSteps(snippet) ? t("snippets.stepCount", { count: snippet.steps?.length ?? 0 }) : snippet.command.split("\n")[0] ?? "";
}

// newStep is the step a person adds: it types its text with Enter, the way a command is typed.
export function newStep(send = ""): SnippetStep {
  return { wait: "", send, enter: true };
}

// stepsCanBeSaved reports whether the engine takes these steps: at least one, and each types something.
export function stepsCanBeSaved(steps: readonly SnippetStep[]): boolean {
  return steps.length > 0 && steps.every((step) => step.send !== "" || step.enter);
}
