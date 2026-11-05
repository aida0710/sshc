import { describe, expect, it } from "vitest";
import { isForwardedToRunTarget, withinForwardedRequestLimit } from "./forwardedRequestLimit";

// A request held open by a host that does not answer, until the test lets it end.
function heldRequest() {
  let finish = () => {};
  const finished = new Promise<void>((resolve) => { finish = resolve; });
  return { send: () => finished, finish };
}

// Lets promises that are already due run.
const settle = () => new Promise((resolve) => setTimeout(resolve));

describe("withinForwardedRequestLimit", () => {
  it("leaves the browser's other connections to this machine's requests while a host does not answer", async () => {
    const held = [heldRequest(), heldRequest(), heldRequest()];
    const answers = held.map((request) => withinForwardedRequestLimit(undefined, request.send));

    let sent = false;
    const waiting = withinForwardedRequestLimit(undefined, async () => { sent = true; });
    await settle();
    expect(sent).toBe(false);

    held[0]!.finish();
    await settle();
    expect(sent).toBe(true);

    held[1]!.finish();
    held[2]!.finish();
    await Promise.all([...answers, waiting]);
  });

  it("never sends a request aborted while it waits", async () => {
    const held = [heldRequest(), heldRequest(), heldRequest()];
    const answers = held.map((request) => withinForwardedRequestLimit(undefined, request.send));
    const leaving = new AbortController();

    let sent = false;
    const waiting = withinForwardedRequestLimit(leaving.signal, async () => { sent = true; });
    leaving.abort();
    await expect(waiting).rejects.toBeDefined();

    held.forEach((request) => request.finish());
    await Promise.all(answers);
    await settle();
    expect(sent).toBe(false);
  });

  it("gives the connection back when a request fails", async () => {
    await expect(withinForwardedRequestLimit(undefined, () => Promise.reject(new Error("no answer")))).rejects.toThrow("no answer");
    const held = [heldRequest(), heldRequest()];
    const answers = held.map((request) => withinForwardedRequestLimit(undefined, request.send));
    let sent = false;
    await withinForwardedRequestLimit(undefined, async () => { sent = true; });
    expect(sent).toBe(true);
    held.forEach((request) => request.finish());
    await Promise.all(answers);
  });
});

describe("isForwardedToRunTarget", () => {
  it("counts what crosses SSH and not the list of run targets, which says how the connection is going", () => {
    expect(isForwardedToRunTarget("/api/v1/engines/r100/terminal/sessions")).toBe(true);
    expect(isForwardedToRunTarget("/api/v1/engines")).toBe(false);
    expect(isForwardedToRunTarget("/api/v1/terminal/sessions")).toBe(false);
    expect(isForwardedToRunTarget("/api/v1/config/overview")).toBe(false);
  });
});
