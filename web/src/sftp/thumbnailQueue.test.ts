import { beforeEach, describe, expect, it, vi } from "vitest";
import { sftpApi } from "./api";
import { requestThumbnail } from "./thumbnailQueue";

type Asked = { path: string; signal: AbortSignal; resolve: (blob: Blob) => void; reject: (error: unknown) => void };

let asked: Asked[];

function request(path: string, finished: (url: string | null) => void = () => undefined) {
  return requestThumbnail({ alias: "edge", path, revision: "r" }, finished);
}

async function settle() {
  await vi.waitFor(() => undefined);
  await new Promise((resolve) => { setTimeout(resolve, 0); });
}

describe("requestThumbnail", () => {
  beforeEach(async () => {
    asked = [];
    vi.spyOn(sftpApi, "thumbnail").mockImplementation(({ path }, signal) => new Promise((resolve, reject) => {
      asked.push({ path, signal, resolve, reject });
    }));
    await settle();
  });

  it("asks for two at a time and starts the next as one finishes", async () => {
    const finished = vi.fn();
    const stops = [request("/a.png", finished), request("/b.png"), request("/c.png")];
    expect(asked.map(({ path }) => path)).toEqual(["/a.png", "/b.png"]);

    asked[0]?.resolve(new Blob(["x"], { type: "image/jpeg" }));
    await vi.waitFor(() => expect(finished).toHaveBeenCalledWith(expect.stringMatching(/^data:image\/jpeg/u)));
    await vi.waitFor(() => expect(asked.map(({ path }) => path)).toContain("/c.png"));

    for (const stop of stops) stop();
    for (const { reject } of asked) reject(new DOMException("aborted", "AbortError"));
    await settle();
  });

  it("never sends one cancelled while it waits", async () => {
    const stops = [request("/a.png"), request("/b.png")];
    const waiting = request("/c.png");
    waiting();
    for (const stop of stops) stop();
    for (const { reject } of asked) reject(new DOMException("aborted", "AbortError"));
    await settle();
    expect(asked.map(({ path }) => path)).toEqual(["/a.png", "/b.png"]);
  });

  it("stops one the engine is making, and does not call back", async () => {
    const finished = vi.fn();
    const stop = request("/a.png", finished);
    stop();
    expect(asked[0]?.signal.aborted).toBe(true);
    asked[0]?.reject(new DOMException("aborted", "AbortError"));
    await settle();
    expect(finished).not.toHaveBeenCalled();
  });

  it("answers null for a file with no picture", async () => {
    const finished = vi.fn();
    request("/a.png", finished);
    asked[0]?.reject(new Error("sftp_preview_type"));
    await vi.waitFor(() => expect(finished).toHaveBeenCalledWith(null));
  });
});
