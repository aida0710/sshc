import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { watchConnectionAttempt } from "./sftpConnectionAttempt";

const api = vi.hoisted(() => ({ connectionAttempt: vi.fn(), answerHostKey: vi.fn() }));
vi.mock("./api", () => ({ sftpApi: api }));

const reading = { log: ["[sshc] line"], finished: false };

describe("watchConnectionAttempt", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    api.connectionAttempt.mockReset();
    api.answerHostKey.mockReset();
  });
  afterEach(() => vi.useRealTimers());

  it("reads the attempt every half second until it is stopped", async () => {
    api.connectionAttempt.mockResolvedValue(reading);
    const onChange = vi.fn();
    const watch = watchConnectionAttempt("edge", onChange);
    expect(watch.attempt).toMatch(/^[A-Za-z0-9-]{16,64}$/);

    await vi.advanceTimersByTimeAsync(1000);
    expect(api.connectionAttempt).toHaveBeenCalledTimes(2);
    expect(api.connectionAttempt).toHaveBeenCalledWith("edge", watch.attempt);
    expect(onChange).toHaveBeenCalledWith(reading);

    await watch.stop({ readFinal: false });
    await vi.advanceTimersByTimeAsync(2000);
    expect(api.connectionAttempt).toHaveBeenCalledTimes(2);
  });

  it("reads once more when stopped for a failure, so the reason is shown", async () => {
    api.connectionAttempt.mockResolvedValue({ ...reading, finished: true });
    const onChange = vi.fn();
    const watch = watchConnectionAttempt("edge", onChange);
    await watch.stop({ readFinal: true });
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ ...reading, finished: true });
  });

  it("waits quietly while the listing has not begun connecting or the read fails", async () => {
    api.connectionAttempt.mockResolvedValueOnce(null).mockRejectedValueOnce(new Error("offline"));
    const onChange = vi.fn();
    const watch = watchConnectionAttempt("edge", onChange);
    await vi.advanceTimersByTimeAsync(1000);
    expect(onChange).not.toHaveBeenCalled();
    await watch.stop({ readFinal: false });
  });

  it("answers the host key question of its attempt and shows the attempt as it is after the answer", async () => {
    const answered = { ...reading, progress: { phase: "authenticating", alias: "edge", hostName: "edge", user: "ops", hop: 1, hops: 1 } };
    api.answerHostKey.mockResolvedValue(answered);
    const onChange = vi.fn();
    const watch = watchConnectionAttempt("edge", onChange);

    await expect(watch.answerHostKey("SHA256:key", true)).resolves.toBe(true);
    expect(api.answerHostKey).toHaveBeenCalledExactlyOnceWith("edge", watch.attempt, { fingerprint: "SHA256:key", accept: true });
    expect(onChange).toHaveBeenCalledExactlyOnceWith(answered);
    await watch.stop({ readFinal: false });
  });

  it("reads the attempt again when the question had closed before the answer", async () => {
    api.answerHostKey.mockResolvedValue(null);
    api.connectionAttempt.mockResolvedValue(reading);
    const onChange = vi.fn();
    const watch = watchConnectionAttempt("edge", onChange);

    await expect(watch.answerHostKey("SHA256:key", false)).resolves.toBe(false);
    expect(onChange).toHaveBeenCalledExactlyOnceWith(reading);
    await watch.stop({ readFinal: false });
  });
});
