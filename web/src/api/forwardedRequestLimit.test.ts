import { describe, expect, it } from "vitest";
import { carriesFile, isForwardedToRunTarget, withinForwardedRequestLimit } from "./forwardedRequestLimit";

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

describe("requests that carry a file", () => {
  it("leave one forwarded connection to the short requests, however many parts of a file are waiting", async () => {
    const parts = [heldRequest(), heldRequest(), heldRequest(), heldRequest()];
    const uploads = parts.map((part) => withinForwardedRequestLimit(undefined, part.send, true));

    let listed = false;
    const list = withinForwardedRequestLimit(undefined, async () => { listed = true; });
    await settle();
    expect(listed).toBe(true);

    parts.forEach((part) => part.finish());
    await Promise.all([...uploads, list]);
  });

  it("go on one after another as the parts before them end", async () => {
    const sent: number[] = [];
    const parts = [0, 1, 2].map((index) => {
      const held = heldRequest();
      return { finish: held.finish, answer: withinForwardedRequestLimit(undefined, () => { sent.push(index); return held.send(); }, true) };
    });
    await settle();
    expect(sent).toEqual([0, 1]);

    parts[0]!.finish();
    await settle();
    expect(sent).toEqual([0, 1, 2]);

    parts[1]!.finish();
    parts[2]!.finish();
    await Promise.all(parts.map((part) => part.answer));
  });

  it("are the uploads' parts and what is read from a file, not the lists and the plans", () => {
    expect(carriesFile("PATCH", "/api/v1/engines/r100/sftp/web/uploads/u1?path=%2Fsrv%2Fa.bin&offset=0")).toBe(true);
    expect(carriesFile("GET", "/api/v1/engines/r100/sftp/web/download?path=%2Fsrv%2Fa.bin")).toBe(true);
    expect(carriesFile("GET", "/api/v1/engines/r100/sftp/local/thumbnail?path=%2Fhome%2Fa.png")).toBe(true);
    expect(carriesFile("POST", "/api/v1/engines/r100/sftp/web/uploads/u1")).toBe(false);
    expect(carriesFile("GET", "/api/v1/engines/r100/sftp/web/entries?path=%2Fsrv")).toBe(false);
    expect(carriesFile("GET", "/api/v1/engines/r100/sftp/transfers")).toBe(false);
    expect(carriesFile("GET", "/api/v1/engines/r100/terminal/sessions")).toBe(false);
    expect(carriesFile("GET", "/api/v1/sftp/web/download?path=%2Fsrv%2Fa.bin")).toBe(false);
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
