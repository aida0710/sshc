import { createContext, useContext, useLayoutEffect, type ReactNode } from "react";
import { reducedMotionQuery } from "../ui/reducedMotion";
import { useMediaQuery } from "../ui/useMediaQuery";

const ReducedMotionContext = createContext<boolean | null>(null);

// MotionProvider keeps <html data-motion="reduce"> while the OS asks to reduce motion (prefers-reduced-motion), for
// index.css and its motion-reduce variant, and gives the same answer to components that animate by script
// (useReducedMotion).
export function MotionProvider({ children }: { children: ReactNode }) {
  const reduced = useMediaQuery(reducedMotionQuery);

  // A layout effect, so the attribute is on <html> before the first paint that could start an animation.
  useLayoutEffect(() => {
    const root = document.documentElement;
    if (reduced) root.dataset.motion = "reduce";
    else delete root.dataset.motion;
  }, [reduced]);

  return <ReducedMotionContext.Provider value={reduced}>{children}</ReducedMotionContext.Provider>;
}

// useReducedMotion is whether to leave animation out. Outside MotionProvider (a component tested alone) it
// follows the OS.
export function useReducedMotion(): boolean {
  const provided = useContext(ReducedMotionContext);
  const systemReducesMotion = useMediaQuery(reducedMotionQuery);
  return provided ?? systemReducesMotion;
}
