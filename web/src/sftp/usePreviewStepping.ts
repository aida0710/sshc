import { useEffect } from "react";
import { insideKeyboardOwner } from "../ui/useDismissibleLayer";
import type { PreviewSwipe } from "./previewSwipe";

// usePreviewStepping moves a file preview through the files around the shown one (count of them, the shown
// one at position): the arrow keys and a finger's swipe go to the previous and the next, and a swipe down
// closes the preview. It returns what the picture's swipes are given.
//
// The arrows are read from the whole document: a step button that just became disabled gives up focus, and
// the keys must keep working after it. Inside the text preview they move the editor's cursor instead.
export function usePreviewStepping({ position, count, onStep, onClose }: {
  position: number;
  count: number;
  onStep: (position: number) => void;
  onClose: () => void;
}) {
  const hasPrevious = position > 0;
  const hasNext = position < count - 1;

  useEffect(() => {
    const step = (event: KeyboardEvent) => {
      if (insideKeyboardOwner(event.target)) return;
      if (event.key === "ArrowLeft" && hasPrevious) onStep(position - 1);
      else if (event.key === "ArrowRight" && hasNext) onStep(position + 1);
      else return;
      event.preventDefault();
    };
    document.addEventListener("keydown", step);
    return () => document.removeEventListener("keydown", step);
  }, [position, hasPrevious, hasNext, onStep]);

  return function onSwipe(swipe: PreviewSwipe) {
    if (swipe === "close") onClose();
    else if (swipe === "previous" && hasPrevious) onStep(position - 1);
    else if (swipe === "next" && hasNext) onStep(position + 1);
  };
}
