import type { Locator } from "@playwright/test";

// pressHeight measures how tall the place to press a control is, by asking the page what a finger at each height
// would land on. A control's box does not count what its ::before or ::after reach over.
export async function pressHeight(control: Locator): Promise<number> {
  return control.evaluate((element) => {
    const box = element.getBoundingClientRect();
    const x = box.left + box.width / 2;
    const middle = box.top + box.height / 2;
    const landsOnControl = (y: number) => {
      const hit = document.elementFromPoint(x, y);
      return hit !== null && (hit === element || element.contains(hit));
    };
    let top = middle;
    while (landsOnControl(top - 1)) top -= 1;
    let bottom = middle;
    while (landsOnControl(bottom + 1)) bottom += 1;
    return Math.round(bottom - top);
  });
}
