import type { Page } from "@playwright/test";

// Presses `shortcut`, such as "Control+F", and resolves to true when the page
// called preventDefault on it, so the browser did not act on the key itself.
// For Ctrl+F, the browser would otherwise open its own Find bar.
export async function pressShortcutAndReportPrevented(page: Page, shortcut: string): Promise<boolean> {
  const key = shortcut.slice(shortcut.lastIndexOf("+") + 1).toLowerCase();
  // Inside an object, so that evaluateHandle returns before the shortcut is
  // pressed instead of waiting for the answer.
  const keyDown = await page.evaluateHandle((key) => ({
    defaultPrevented: new Promise<boolean>((resolve) => {
      const listener = (event: KeyboardEvent) => {
        // The modifier keys go down first.
        if (event.key.toLowerCase() !== key) return;
        window.removeEventListener("keydown", listener, true);
        // Read once every listener on the page has had the key.
        setTimeout(() => resolve(event.defaultPrevented));
      };
      window.addEventListener("keydown", listener, true);
    }),
  }), key);
  await page.keyboard.press(shortcut);
  return keyDown.evaluate((pending) => pending.defaultPrevented);
}
