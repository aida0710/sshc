import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { sftpApi } from "./api";

describe("sftpApi resumable download", () => {
  beforeEach(() => {
    apiClient.setCSRF("a".repeat(43));
    vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:test");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
  });

  afterEach(() => {
    apiClient.clear();
    vi.restoreAllMocks();
  });

  it("continues a failed file stream with an HTTP Range request", async () => {
    let reads = 0;
    const first = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (reads++ === 0) controller.enqueue(new TextEncoder().encode("abc"));
        else controller.error(new Error("connection_lost"));
      },
    });
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(first, { status: 200, headers: { "Content-Length": "6" } }))
      .mockResolvedValueOnce(new Response("def", { status: 206, headers: { "Content-Length": "3", "Content-Range": "bytes 3-5/6" } }));

    const progress: number[] = [];
    const bytes = await sftpApi.download("edge", "/remote/file.bin", false, { onProgress: (value) => progress.push(value) });

    expect(bytes).toBe(6);
    expect(progress).toEqual([3, 6]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const second = fetchMock.mock.calls[1]?.[1];
    expect(new Headers(second?.headers).get("Range")).toBe("bytes=3-");
  });
});
