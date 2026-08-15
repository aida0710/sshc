"use strict";

const { EventEmitter } = require("node:events");
const { test } = require("node:test");
const assert = require("node:assert/strict");
const { installWindowReopener } = require("./reopen");

function windowRecorder({ minimized = false } = {}) {
  const calls = [];
  return {
    calls,
    window: {
      isMinimized: () => minimized,
      restore: () => calls.push("restore"),
      show: () => calls.push("show"),
      focus: () => calls.push("focus"),
    },
  };
}

test("a second launch during startup restores and focuses the first window once ready", async () => {
  const app = new EventEmitter();
  const existing = windowRecorder({ minimized: true });
  const reopener = installWindowReopener({
    app,
    getWindows: () => [existing.window],
    createWindow: async () => assert.fail("must reuse the existing window"),
    showFailure: assert.fail,
  });

  app.emit("second-instance");
  assert.deepEqual(existing.calls, []);

  await reopener.start();
  assert.deepEqual(existing.calls, ["restore", "show", "focus"]);
});

test("activation shows and focuses an existing window without restoring a normal window", async () => {
  const app = new EventEmitter();
  const existing = windowRecorder();
  const reopener = installWindowReopener({
    app,
    getWindows: () => [existing.window],
    createWindow: async () => assert.fail("must reuse the existing window"),
    showFailure: assert.fail,
  });
  await reopener.start();

  app.emit("activate");
  await new Promise((resolve) => setImmediate(resolve));

  assert.deepEqual(existing.calls, ["show", "focus"]);
});

test("concurrent reopen requests create at most one window", async () => {
  const app = new EventEmitter();
  let release;
  const opened = new Promise((resolve) => { release = resolve; });
  let creations = 0;
  const reopener = installWindowReopener({
    app,
    getWindows: () => [],
    createWindow: async () => {
      creations += 1;
      await opened;
    },
    showFailure: assert.fail,
  });
  await reopener.start();

  const first = reopener.request();
  const second = reopener.request();
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(creations, 1);

  release();
  await Promise.all([first, second]);
  assert.equal(creations, 1);
});

test("a failure to open another window is reported without rejecting the event handler", async () => {
  const app = new EventEmitter();
  const failure = new Error("no entrance");
  const reported = [];
  const reopener = installWindowReopener({
    app,
    getWindows: () => [],
    createWindow: async () => { throw failure; },
    showFailure: (error) => reported.push(error),
  });
  await reopener.start();

  await reopener.request();

  assert.deepEqual(reported, [failure]);
});
