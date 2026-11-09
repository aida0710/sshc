// A spring finishes what a finger let go of: it leaves at the speed the finger was moving and comes to rest on its
// target, so a fling lands sooner than a slow let-go, and a finger that lands on the way can stop it where it is
// (stop). A move of fixed length would make every fling land the same.

// How hard the spring pulls toward its target, per second squared: at this pull it rests in about a third of a
// second, as Android's drawers do.
const stiffness = 400;
// Critical damping: the spring comes to rest without swinging back and forth.
const damping = 2 * Math.sqrt(stiffness);
// Close enough to the target, slowly enough, the spring stops on it (pixels, pixels a second).
const restDistance = 0.5;
const restSpeed = 10;
// A frame after a pause (the page hidden for a moment) moves no further than this, so the spring does not jump.
const longestStepMilliseconds = 32;

export type SpringRun = {
  // stop ends the run where it is and returns that position.
  stop(): number;
};

type SpringOptions = {
  from: number;
  to: number;
  // velocity is the speed the finger left with, in pixels a millisecond.
  velocity: number;
  onFrame: (position: number) => void;
  onRest: () => void;
  // reducedMotion lands at once, without the run.
  reducedMotion?: boolean;
};

export function runSpring({ from, to, velocity, onFrame, onRest, reducedMotion = false }: SpringOptions): SpringRun {
  if (reducedMotion) {
    onFrame(to);
    onRest();
    return { stop: () => to };
  }
  let position = from;
  let speed = velocity * 1000;
  let last = performance.now();
  let frame = requestAnimationFrame(step);

  function step(time: number) {
    const seconds = Math.min(longestStepMilliseconds, Math.max(0, time - last)) / 1000;
    last = time;
    speed += (-stiffness * (position - to) - damping * speed) * seconds;
    position += speed * seconds;
    if (Math.abs(position - to) < restDistance && Math.abs(speed) < restSpeed) {
      position = to;
      onFrame(to);
      onRest();
      return;
    }
    onFrame(position);
    frame = requestAnimationFrame(step);
  }

  return {
    stop() {
      cancelAnimationFrame(frame);
      return position;
    },
  };
}
