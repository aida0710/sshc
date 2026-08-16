"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const { spawnEngine, releaseEngine } = require("./engine");

// **stdin は開いたパイプでなければならない。** それがこのアプリの所有権その
// ものである。`ignore` を渡すと engine には /dev/null が届き、所有者の居ない
// 起動として断られる——窓は一度も開かない。
test("Electron starts its child through the explicit engine invocation", () => {
  for (const binary of [
    "/Applications/sshc.app/Contents/Resources/sshc",
    "/workspace/sshc/bin/sshc",
  ]) {
    const calls = [];
    const child = { stdout: {} };
    const spawned = spawnEngine((calledBinary, args, options) => {
      calls.push({ binary: calledBinary, args, options });
      return child;
    }, binary);

    assert.strictEqual(spawned, child);
    assert.deepEqual(calls, [
      {
        binary,
        args: ["engine"],
        options: { stdio: ["pipe", "pipe", "inherit"] },
      },
    ]);
    assert.ok(!calls[0].args.includes("--own-engine"));
  }
});

test("releasing the engine closes the ownership channel instead of killing it", () => {
  const events = {};
  let ended = false;
  let killed = null;
  const child = {
    exitCode: null,
    signalCode: null,
    stdin: {
      end: () => {
        ended = true;
      },
    },
    kill: (signal) => {
      killed = signal;
    },
    once: (name, listener) => {
      events[name] = listener;
    },
  };

  releaseEngine(child, 5000);

  assert.ok(ended, "the ownership channel was not closed");
  assert.strictEqual(
    killed,
    null,
    "the engine was killed instead of being released",
  );
  assert.ok(typeof events.exit === "function", "the exit was not observed");
  events.exit();
});

test("releasing an engine that has already exited does nothing", () => {
  let touched = false;
  const child = {
    exitCode: 0,
    signalCode: null,
    stdin: {
      end: () => {
        touched = true;
      },
    },
    kill: () => {
      touched = true;
    },
    once: () => {
      touched = true;
    },
  };
  releaseEngine(child, 5000);
  assert.ok(!touched, "an already exited engine was touched");
});

test("an engine that ignores the closed channel is stopped by the deadline", async () => {
  let killed = null;
  const child = {
    exitCode: null,
    signalCode: null,
    stdin: { end: () => {} },
    kill: (signal) => {
      killed = signal;
    },
    once: () => {},
  };

  releaseEngine(child, 1);
  await new Promise((done) => setTimeout(done, 20));
  assert.strictEqual(killed, "SIGKILL");
});
