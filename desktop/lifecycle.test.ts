import assert from "node:assert/strict";
import { test } from "node:test";
import type { SpawnOptions } from "node:child_process";
import {
  engineSpawnOptions,
  spawnEngine,
  stopOwnedEngine,
  shouldQuitAfterLastWindow,
} from "./lifecycle.js";

/** stubChild は、spawn が返す子の最小の姿を作る。 */
type StubChild = {
  exitCode: number | null;
  signalCode: string | null;
  ended: boolean;
  killed: string | null;
  stdout: Record<string, never>;
  stderr: { on(name: string, handler: (chunk: Buffer | string) => unknown): unknown };
  stdin: { end(): void };
  kill(signal: "SIGKILL"): void;
  once(name: string, listener: () => unknown): void;
  emit(name: string): void;
};

function stubChild(): StubChild {
  const listeners: Record<string, (() => unknown) | undefined> = {};
  const child: StubChild = {
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
      child.killed = signal;
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
    const calls: {
      binary: string;
      args: readonly string[];
      options: SpawnOptions;
    }[] = [];
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
    assert.ok(!calls[0]?.args.includes("--own-engine"));
  }
});

// **stdin は engine の object の外へ出さない。** 所有権のチャンネルを renderer
// や helper が触れる場所へ置けば、画面の側からアプリの寿命を終わらせられる。
test("the spawn options expose no channel beyond the three standard streams", () => {
  const options = engineSpawnOptions();
  assert.deepEqual(Object.keys(options).sort(), ["stdio", "windowsHide"]);
  const stdio = options.stdio as readonly string[];
  assert.strictEqual(stdio.length, 3);
  assert.strictEqual(stdio[0], "pipe");
});

// **stderr はパイプだが読み続けなければならない。** 読まずに置くと 64 KiB で
// 埋まり、その先で止まるのは write を呼んだ engine 自身になる——症状は
// 「アプリが黙って固まる」であって、原因が engine に見える壊れ方をする。
test("the engine's stderr is drained instead of being left to fill", () => {
  const written: string[] = [];
  const child = stubChild();
  let listener: ((chunk: Buffer | string) => unknown) | null = null;
  child.stderr = {
    on: (name: string, handler: (chunk: Buffer | string) => unknown) => {
      if (name === "data") listener = handler;
    },
  };

  spawnEngine(() => child, "/bin/sshc", {
    write: (chunk: Buffer | string) => written.push(String(chunk)),
  });

  // **`as` はここでは飾りではない。** listener に入るのは spawnEngine が
  // 呼ぶ callback の中でだけなので、TS はこの行の時点で「まだ null しか
  // 入っていない」と読む——そのままだと次の行の絞り込みが never になる。
  const reader = listener as ((chunk: Buffer | string) => unknown) | null;
  assert.ok(reader !== null, "nothing is reading the engine's stderr");
  reader("could not take the engine lock\n");
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
