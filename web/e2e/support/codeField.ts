import type { Locator, Page } from "@playwright/test";

// A CodeField (src/editor/CodeField.tsx) is a Monaco editor: Monaco keeps the text in its own model and draws only
// the lines in view, and its text area holds only a little of the text around the caret. So tests write into it with
// the keyboard, as a person does, and read the lines it draws.

// codeField is the frame of the CodeField whose name matches.
export function codeField(page: Page, name: string | RegExp): Locator {
  return page.locator("[data-code-field]").filter({ has: page.getByRole("textbox", { name }) });
}

// codeFieldLines are the lines the field draws, for reading what it holds.
export function codeFieldLines(field: Locator): Locator {
  return field.locator(".view-lines");
}

// replaceCodeFieldText puts text in place of everything the field holds, as pasting it would.
export async function replaceCodeFieldText(field: Locator, text: string): Promise<void> {
  await field.locator(".view-lines").click();
  await field.page().keyboard.press("ControlOrMeta+A");
  await field.page().keyboard.press("Delete");
  await field.page().keyboard.insertText(text);
}

// replaceCodeFieldLine puts text in place of the first line that holds find, from its first character that is not
// a space or tab.
export async function replaceCodeFieldLine(field: Locator, find: string, text: string): Promise<void> {
  await field.locator(".view-line", { hasText: find }).first().click();
  await field.page().keyboard.press("Home");
  await field.page().keyboard.press("Shift+End");
  await field.page().keyboard.insertText(text);
}

// appendCodeFieldText adds text after the field's last character.
export async function appendCodeFieldText(field: Locator, text: string): Promise<void> {
  await field.locator(".view-lines").click();
  await field.page().keyboard.press("ControlOrMeta+End");
  await field.page().keyboard.insertText(text);
}

// pageAlert is what the page itself says as an alert. Once an editor has been shown, Monaco keeps two empty
// role="alert" regions of its own on the page, for screen readers to announce through; they are left out.
export function pageAlert(page: Page): Locator {
  return page.locator('[role="alert"]:not(.monaco-alert)');
}
