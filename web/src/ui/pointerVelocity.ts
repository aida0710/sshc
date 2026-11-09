// A pointer's speed as it moves, in pixels a millisecond, followed over its last moves and weighted toward the
// latest, so that one uneven step does not decide a fling.
export type PointerVelocity = { x: number; y: number };

export type VelocityTracker = {
  // move records where the pointer is at time (milliseconds).
  move(x: number, y: number, time: number): void;
  // velocity is the speed now; a pointer that has rested longer than a fling's pause has none.
  velocity(time: number): PointerVelocity;
};

// A pause this long before the finger lifts means it stopped: the let-go carries no speed.
const restingPauseMilliseconds = 80;
// How much of the speed before a move is kept beside the move's own speed.
const keptShare = 0.4;

export function newVelocityTracker(x: number, y: number, time: number): VelocityTracker {
  let last = { x, y, time };
  let speed: PointerVelocity = { x: 0, y: 0 };
  return {
    move(nextX, nextY, nextTime) {
      const elapsed = nextTime - last.time;
      if (elapsed <= 0) return;
      const step = { x: (nextX - last.x) / elapsed, y: (nextY - last.y) / elapsed };
      speed = { x: speed.x * keptShare + step.x * (1 - keptShare), y: speed.y * keptShare + step.y * (1 - keptShare) };
      last = { x: nextX, y: nextY, time: nextTime };
    },
    velocity(time) {
      return time - last.time > restingPauseMilliseconds ? { x: 0, y: 0 } : speed;
    },
  };
}
