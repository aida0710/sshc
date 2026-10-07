import assert from "node:assert/strict";
import test from "node:test";
import { VMBridge } from "../src/vm-bridge.js";
import { connectVirtualLAN } from "../src/vm-network.js";

function fakeMachine() {
  const listeners = new Map();
  const received = [];
  return {
    received,
    bus: { send: (kind, bytes) => received.push({ kind, bytes }) },
    add_listener: (kind, listener) => listeners.set(kind, listener),
    emit: (kind, bytes) => listeners.get(kind)?.(bytes),
  };
}

test("VMのEthernetフレームは同じデモ内の接続先だけに届く", () => {
  const machines = [fakeMachine(), fakeMachine(), fakeMachine()];
  const anotherDemo = [fakeMachine(), fakeMachine(), fakeMachine()];
  connectVirtualLAN(machines);
  connectVirtualLAN(anotherDemo);
  const frame = Uint8Array.of(1, 2, 3);
  machines[0].emit("net0-send", frame);
  assert.equal(machines[0].received.length, 0);
  assert.ok(anotherDemo.every((machine) => machine.received.length === 0));
  assert.deepEqual(machines[1].received[0], { kind: "net0-receive", bytes: frame });
  machines[1].received[0].bytes[0] = 9;
  assert.equal(frame[0], 1);
  assert.equal(machines[2].received[0].bytes[0], 1);
});

test("シリアル通信をまたいでも日本語のJSON応答が崩れない", () => {
  const machine = fakeMachine();
  const bridge = new VMBridge(machine);
  const replies = [];
  bridge.subscribe((reply) => replies.push(reply));
  const reply = { id: "1", kind: "message", text: "日本語のファイル.txt" };
  for (const byte of new TextEncoder().encode(JSON.stringify(reply) + "\n")) {
    machine.emit("serial1-output-byte", byte);
  }
  assert.deepEqual(replies, [reply]);
  bridge.send({ kind: "fetch", path: "/api/v1/日本語" });
  assert.ok(machine.received.every((entry) => entry.kind === "serial1-input"));
  assert.equal(new TextDecoder().decode(Uint8Array.from(machine.received.map((entry) => entry.bytes))),
    JSON.stringify({ kind: "fetch", path: "/api/v1/日本語" }) + "\n");
});
