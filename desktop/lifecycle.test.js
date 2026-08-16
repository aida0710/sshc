"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const {
  engineSpawnOptions,
  spawnEngine,
  stopOwnedEngine,
  shouldQuitAfterLastWindow,
} = require("./lifecycle");

/** stubChild は、spawn が返す子の最小の姿を作る。 */
function stubChild() {
  const listeners = {};
  const child = {
    exitCode: null,
    signalCode: null,
    ended: false,
    killed: null,
    stdout: {},
    stderr: { on: () => {} },
    stdin: {
      end: () => {
        child.ended = true;
      },
    },
    kill(signal) {
      this.killed = signal;
    },
    once(name, listener) {
      listeners[name] = listener;
    },
    emit(name) {
      listeners[name]?.();
    },
  };
  return child;
}

// **stdin は開いたパイプでなければならない。** それがこのアプリの所有権その
// ものである。`ignore` を渡すと engine には /dev/null が届き、所有者の居ない
// 起動として断られる——窓は一度も開かない。
test("Electron starts its child through the explicit engine invocation", () => {
  for (const binary of [
    "/Applications/sshc.app/Contents/Resources/sshc",
    "C:\\Users\\a\\AppData\\Local\\Programs\\sshc\\resources\\cli\\sshc.exe",
  ]) {
    const calls = [];
    const child = stubChild();
    const spawned = spawnEngine((calledBinary, args, options) => {
      calls.push({ binary: calledBinary, args, options });
      return child;
    }, binary);

    assert.strictEqual(spawned, child);
    assert.deepEqual(calls, [
      {
        binary,
        args: ["engine"],
        options: {
          stdio: ["pipe", "pipe", "pipe"],
          windowsHide: true,
        },
      },
    ]);
    assert.ok(!calls[0].args.includes("--own-engine"));
  }
});

// **stdin は engine の object の外へ出さない。** 所有権のチャンネルを renderer
// や helper が触れる場所へ置けば、画面の側からアプリの寿命を終わらせられる。
test("the spawn options expose no channel beyond the three standard streams", () => {
  const options = engineSpawnOptions();
  assert.deepEqual(Object.keys(options).sort(), ["stdio", "windowsHide"]);
  assert.strictEqual(options.stdio.length, 3);
  assert.strictEqual(options.stdio[0], "pipe");
});

// **stderr はパイプだが読み続けなければならない。** 読まずに置くと 64 KiB で
// 埋まり、その先で止まるのは write を呼んだ engine 自身になる——症状は
// 「アプリが黙って固まる」であって、原因が engine に見える壊れ方をする。
test("the engine's stderr is drained instead of being left to fill", () => {
  const written = [];
  const child = stubChild();
  let listener = null;
  child.stderr = {
    on: (name, handler) => {
      if (name === "data") listener = handler;
    },
  };

  spawnEngine(() => child, "/bin/sshc", {
    write: (chunk) => written.push(String(chunk)),
  });

  assert.ok(listener !== null, "nothing is reading the engine's stderr");
  listener("could not take the engine lock\n");
  assert.deepEqual(written, ["could not take the engine lock\n"]);
});

test("stopping the engine closes the ownership channel instead of killing it", async () => {
  const child = stubChild();

  const stopping = stopOwnedEngine(child, 5000);
  assert.ok(child.ended, "the ownership channel was not closed");
  assert.strictEqual(
    child.killed,
    null,
    "the engine was killed instead of being released",
  );

  child.emit("exit");
  await stopping;
});

test("stopping an engine that has already exited does nothing", async () => {
  const child = stubChild();
  child.exitCode = 0;

  await stopOwnedEngine(child, 5000);

  assert.ok(!child.ended, "an already exited engine was touched");
  assert.strictEqual(child.killed, null);
});

// 通常の終わり方ではない。それでも期限が無ければ、閉じたチャンネルを無視する
// engine が終了そのものを人質に取れる。
test("an engine that ignores the closed channel is stopped by the deadline", async () => {
  const child = stubChild();

  await stopOwnedEngine(child, 1);

  assert.strictEqual(child.killed, "SIGKILL");
});

// **窓を閉じることは、終わらせる意思表示ではない。** 解錠済みの vault も、
// 開いている SSH も、動いている端末も、窓より長く生きる。
test("closing the last window never quits, on any platform or tray outcome", () => {
  assert.strictEqual(shouldQuitAfterLastWindow(), false);
});
