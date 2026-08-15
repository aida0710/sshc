"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const { spawnEngine } = require("./engine");

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
    assert.deepEqual(calls, [{
      binary,
      args: ["engine"],
      options: { stdio: ["ignore", "pipe", "inherit"] },
    }]);
    assert.ok(!calls[0].args.includes("--own-engine"));
  }
});
