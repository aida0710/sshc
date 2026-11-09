// A spring finishes what a finger let go of: it leaves at the speed the finger was moving and comes to rest on its
// target, so a fling lands sooner than a slow let-go, and a finger that lands on the way can stop it where it is
// (stop). A move of fixed length would make every fling land the same.

// How hard the spring pulls toward its target, per second squared: at this pull it rests in about a third of a
// second, as Android's drawers do. The spring is critically damped: it comes to rest without swinging back and forth.
const stiffness = 400;
const angularFrequency = Math.sqrt(stiffness);
// Close enough to the target, slowly enough, the spring stops on it (units, units a second). The units are pixels
// for a place and a hundredth for a scale, which needs a finer stop.
const restDistance = 0.5;
const restSpeed = 10;

// springStep is where a critically damped spring is after seconds, from offset (units from its target) at speed
// (units a second). It is the exact motion, so a frame of any length lands the spring where it would be: summing the
// pull over a long frame (a phone that drops frames) overshot and made the spring shake on its way to rest.
function springStep(offset: number, speed: number, seconds: number): { offset: number; speed: number } {
  const decay = Math.exp(-angularFrequency * seconds);
  const carried = speed + angularFrequency * offset;
  return {
    offset: (offset + carried * seconds) * decay,
    speed: (speed - angularFrequency * carried * seconds) * decay,
  };
}

export type SpringRun = {
  // stop ends the run where it is and returns that position.
  stop(): number;
};

export type SpringsRun = {
  // stop ends the run where it is and returns those positions.
  stop(): number[];
};

// The clock and frames a spring runs on: the page's, unless a caller with its own (touchScroll.ts) hands them in.
export type SpringClock = {
  now: () => number;
  requestFrame: (callback: FrameRequestCallback) => number;
  cancelFrame: (id: number) => void;
};

const pageClock: SpringClock = {
  now: () => performance.now(),
  requestFrame: (callback) => requestAnimationFrame(callback),
  cancelFrame: (id) => cancelAnimationFrame(id),
};

type SpringsOptions = {
  from: readonly number[];
  to: readonly number[];
  // velocity is the speed each value left the finger with, in units a millisecond.
  velocity: readonly number[];
  // scale is how many units of each value make one pixel of rest distance (100 for a zoom factor).
  scale?: readonly number[];
  onFrame: (positions: number[]) => void;
  onRest: () => void;
  // reducedMotion lands at once, without the run.
  reducedMotion?: boolean;
  clock?: SpringClock;
};

// runSprings moves several values together (a picture's place and size), each on its own spring, and rests when all
// of them are at rest.
export function runSprings({ from, to, velocity, scale, onFrame, onRest, reducedMotion = false, clock = pageClock }: SpringsOptions): SpringsRun {
  if (reducedMotion) {
    onFrame([...to]);
    onRest();
    return { stop: () => [...to] };
  }
  const positions = [...from];
  const speeds = velocity.map((speed) => speed * 1000);
  const units = to.map((_, index) => scale?.[index] ?? 1);
  let last = clock.now();
  let frame = clock.requestFrame(step);
  // stopped is set by a stop from inside onFrame, which must not ask for the next frame.
  let stopped = false;

  function step(time: number) {
    const seconds = Math.max(0, time - last) / 1000;
    last = time;
    let resting = true;
    for (let index = 0; index < positions.length; index += 1) {
      const target = to[index] ?? 0;
      const unit = units[index] ?? 1;
      const { offset, speed } = springStep((positions[index] ?? 0) - target, speeds[index] ?? 0, seconds);
      speeds[index] = speed;
      positions[index] = target + offset;
      if (Math.abs(offset) * unit >= restDistance || Math.abs(speed) * unit >= restSpeed) resting = false;
    }
    if (resting) {
      to.forEach((target, index) => {
        positions[index] = target;
      });
      onFrame([...positions]);
      onRest();
      return;
    }
    onFrame([...positions]);
    if (!stopped) frame = clock.requestFrame(step);
  }

  return {
    stop() {
      stopped = true;
      clock.cancelFrame(frame);
      return [...positions];
    },
  };
}

type SpringOptions = {
  from: number;
  to: number;
  // velocity is the speed the finger left with, in pixels a millisecond.
  velocity: number;
  onFrame: (position: number) => void;
  onRest: () => void;
  reducedMotion?: boolean;
  clock?: SpringClock;
};

// runSpring moves one value.
export function runSpring({ from, to, velocity, onFrame, onRest, reducedMotion, clock }: SpringOptions): SpringRun {
  const run = runSprings({
    from: [from],
    to: [to],
    velocity: [velocity],
    onFrame: ([position]) => onFrame(position!),
    onRest,
    ...(reducedMotion === undefined ? {} : { reducedMotion }),
    ...(clock === undefined ? {} : { clock }),
  });
  return { stop: () => run.stop()[0]! };
}
