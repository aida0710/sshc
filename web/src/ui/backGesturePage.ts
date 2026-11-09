// How the page looks while Android's back gesture is drawn (android/backGestureMotion.ts follows the finger with
// drawBackGesture, pageMotion.ts draws the page whole when Back goes to another page): the app shrinks a little,
// rounds its corners and moves away from the edge the finger came from, over a darker ground (pageMotion.css).
// The transform is written on the app's element itself. A variable on <html> is inherited by every element, and
// changing it on each move of the finger made the browser work out the style of the whole page again each time.

// At the end of the gesture's travel: 10% smaller, 16px aside, corners rounded by 28px, as Android draws its own
// screens.
const shrinkAtEnd = 0.1;
const shiftAtEnd = 16;
const cornerAtEnd = 28;
// A gesture taken back grows back to its place quickly, slowing down as it lands.
const settleTransition = "200ms cubic-bezier(0.2, 0, 0, 1)";

function appElement(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".sshc-app");
}

function place(app: HTMLElement, progress: number, edge: "left" | "right") {
  const away = edge === "left" ? 1 : -1;
  app.style.transform = `translateX(${away * progress * shiftAtEnd}px) scale(${1 - progress * shrinkAtEnd})`;
  app.style.borderRadius = `${progress * cornerAtEnd}px`;
}

// drawBackGesture draws the page as far along as the finger is (progress from 0 to 1).
export function drawBackGesture(progress: number, edge: "left" | "right") {
  const app = appElement();
  if (app === null) return;
  document.documentElement.dataset.backGesture = "";
  app.style.transition = "none";
  place(app, Math.min(1, Math.max(0, progress)), edge);
}

// drawPageWhole puts the page back at once and forgets the gesture.
export function drawPageWhole() {
  delete document.documentElement.dataset.backGesture;
  const app = appElement();
  if (app === null) return;
  app.style.transition = "";
  app.style.transform = "";
  app.style.borderRadius = "";
}

// settleBackGesture grows the page back to its place, and forgets the gesture once it is there.
export function settleBackGesture() {
  const app = appElement();
  if (app === null || app.style.transform === "") {
    drawPageWhole();
    return;
  }
  const landed = (event: TransitionEvent) => {
    if (event.target !== app || event.propertyName !== "transform") return;
    app.removeEventListener("transitionend", landed);
    app.removeEventListener("transitioncancel", landed);
    drawPageWhole();
  };
  app.addEventListener("transitionend", landed);
  app.addEventListener("transitioncancel", landed);
  app.style.transition = `transform ${settleTransition}, border-radius ${settleTransition}`;
  place(app, 0, "left");
}
