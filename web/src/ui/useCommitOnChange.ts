import { useEffect, useRef, type RefObject } from "react";

// useCommitOnChange calls commit when a typed field inside the element (text, a number, a box of text) is left
// with a new value, or Enter is pressed in it: the browser's change event. A page that saves each change at
// once saves a typed field there, not on every key, so a half-typed value is never saved. Lists and boxes to
// tick fire change as they are chosen; the page saves those itself.
export function useCommitOnChange(container: RefObject<HTMLElement | null>, commit: () => void) {
  const latestCommit = useRef(commit);
  latestCommit.current = commit;
  useEffect(() => {
    const element = container.current;
    if (element === null) return;
    const changed = (event: Event) => {
      const field = event.target;
      const typed = field instanceof HTMLTextAreaElement
        || (field instanceof HTMLInputElement && ["text", "number", "search"].includes(field.type));
      if (typed) latestCommit.current();
    };
    element.addEventListener("change", changed);
    return () => element.removeEventListener("change", changed);
  }, [container]);
}
