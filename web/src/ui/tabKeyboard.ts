import type { KeyboardEvent } from "react";

// tabsScopeAttribute marks the element around tabs that a tablist owns from outside (aria-owns), for a strip that keeps
// other buttons between its tabs.
const tabsScopeAttribute = "data-tabs-scope";
export const tabsScopeProps = { [tabsScopeAttribute]: "" } as const;

export function activateTabFromKeyboard<T>(
  event: KeyboardEvent<HTMLButtonElement>,
  index: number,
  values: readonly T[],
  onActivate: (value: T) => void,
) {
  let next: number;
  switch (event.key) {
    case "ArrowRight":
      next = (index + 1) % values.length;
      break;
    case "ArrowLeft":
      next = (index - 1 + values.length) % values.length;
      break;
    case "Home":
      next = 0;
      break;
    case "End":
      next = values.length - 1;
      break;
    default:
      return;
  }

  event.preventDefault();
  const tablist = event.currentTarget.closest(`[role="tablist"], [${tabsScopeAttribute}]`);
  const tabs = tablist?.querySelectorAll<HTMLButtonElement>('[role="tab"]');
  tabs?.[next]?.focus();
  const value = values[next];
  if (value !== undefined) onActivate(value);
}
