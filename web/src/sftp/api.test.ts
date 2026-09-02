import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient, whenRequestFailed } from "../api/client";
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
    whenRequestFailed(null);
    vi.restoreAllMocks();
  });

  it("leaves directory-list connection failures to the SFTP panel", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(
      JSON.stringify({ code: "sftp_failed", message: "request rejected" }),
      { status: 502, headers: { "Content-Type": "application/problem+json" } },
    ));

    await expect(sftpApi.list("miyabi", "/")).rejects.toMatchObject({ code: "sftp_failed" });
    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("omits the path query when opening the remote user's initial directory", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      path: "/home/aida", entries: [],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await expect(sftpApi.list("edge")).resolves.toMatchObject({ path: "/home/aida" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/sftp/edge/entries");
  });

  it("uses the engine-provided transfer controls as the UI contract", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      maxConcurrent: 2,
      clearCompletedAfterSeconds: 0,
      processingStopped: false,
      largeFileThresholdBytes: 100 << 20,
      largeFileParallelism: 4,
      largeFileChunkBytes: 32 << 20,
      jobs: [{
        id: "transfer_test01", batchId: "batch_test0001", batchName: "file.bin", batchKind: "file",
        alias: "edge", direction: "download", kind: "file", name: "file.bin", remotePath: "/file.bin",
        sourceAlias: "", sourcePath: "", operation: "", overwrite: false,
        totalBytes: 4, transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1,
        status: "queued", allowedActions: ["pause", "cancel"], attempt: 1, problem: "", lastModified: 0,
        expectedRevision: "", sourceFingerprint: "", downloadRevision: "",
        createdAt: "2026-08-31T00:00:00Z", updatedAt: "2026-08-31T00:00:00Z",
      }],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    const response = await sftpApi.listTransfers();

    expect(response.jobs[0]?.allowedActions).toEqual(["pause", "cancel"]);
  });

  it("removes one retained transfer without escalating an expected cleanup failure", async () => {
    const diagnostic = vi.fn();
    whenRequestFailed(diagnostic);
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ code: "sftp_failed", message: "request rejected" }),
        { status: 502, headers: { "Content-Type": "application/problem+json" } },
      ));

    await expect(sftpApi.removeTransfer("transfer_test01")).resolves.toBeUndefined();
    await expect(sftpApi.removeTransfer("transfer_test02")).rejects.toMatchObject({ code: "sftp_failed" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/sftp/transfers/transfer_test01");
    expect(diagnostic).not.toHaveBeenCalled();
  });

  it("sends only source identity when starting an engine-owned upload", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: "transfer_test01", path: "/remote/file.bin", offset: 0, size: 4, expectedRevision: "absent",
      completedRanges: [], parallelism: 1, chunkBytes: 32 << 20,
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await sftpApi.startUpload("edge", "transfer_test01", "/remote/file.bin", 4, `tree-sha256:${"a".repeat(64)}`);

    const request = fetchMock.mock.calls[0];
    expect(request?.[0]).toBe("/api/v1/sftp/edge/uploads/transfer_test01");
    const body = request?.[1]?.body;
    expect(typeof body).toBe("string");
    expect(JSON.parse(body as string)).toEqual({
      path: "/remote/file.bin", size: 4, sourceFingerprint: `tree-sha256:${"a".repeat(64)}`,
    });
  });

  it("streams one upload range with its declared offset and length", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      id: "transfer_test01", path: "/remote/file.bin", offset: 6, size: 8, expectedRevision: "absent",
      completedRanges: [{ offset: 2, size: 4 }], parallelism: 2, chunkBytes: 8 << 20,
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    const chunk = new Blob(["cdef"]);

    await sftpApi.appendUploadRange("edge", "transfer_test01", "/remote/file.bin", 2, 8, chunk);

    const request = fetchMock.mock.calls[0];
    expect(request?.[0]).toContain("offset=2&total=8&range=true&length=4");
    expect(request?.[1]?.body).toBe(chunk);
  });

  it("streams a resumed file with Range and If-Range", async () => {
    let reads = 0;
    const first = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (reads++ === 0) controller.enqueue(new TextEncoder().encode("abc"));
        else controller.error(new Error("connection_lost"));
      },
    });
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(first, { status: 200, headers: { "Content-Length": "6", ETag: '"revision-1"' } }))
      .mockResolvedValueOnce(new Response("def", { status: 206, headers: { "Content-Length": "3", "Content-Range": "bytes 3-5/6", ETag: '"revision-1"' } }));

    const firstChunks: Uint8Array[] = [];
    await expect(sftpApi.streamDownload("edge", "transfer_test01", "/remote/file.bin", false, 0, {
      onChunk: (chunk) => { firstChunks.push(chunk); },
    })).rejects.toThrow("connection_lost");
    expect(firstChunks.reduce((sum, chunk) => sum + chunk.byteLength, 0)).toBe(3);

    const resumed: Uint8Array[] = [];
    const result = await sftpApi.streamDownload("edge", "transfer_test01", "/remote/file.bin", false, 3, {
      revision: '"revision-1"', onChunk: (chunk) => { resumed.push(chunk); },
    });
    expect(result).toEqual({ bytes: 6, total: 6 });
    expect(new TextDecoder().decode(resumed[0])).toBe("def");
    const second = fetchMock.mock.calls[1]?.[1];
    expect(new Headers(second?.headers).get("Range")).toBe("bytes=3-");
    expect(new Headers(second?.headers).get("If-Range")).toBe('"revision-1"');
  });

  it("discards the old prefix when If-Range reports a replaced remote file", async () => {
    let reads = 0;
    const oldPrefix = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (reads++ === 0) controller.enqueue(new TextEncoder().encode("ABC"));
        else controller.error(new Error("connection_lost"));
      },
    });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(new Response(oldPrefix, {
      status: 200, headers: { "Content-Length": "6", ETag: '"old"' },
    })).mockResolvedValueOnce(new Response("abcxyz", {
      status: 200, headers: { "Content-Length": "6", ETag: '"new"' },
    }));
    await expect(sftpApi.streamDownload("edge", "transfer_test01", "/remote/file.bin", false, 0, { onChunk: () => undefined })).rejects.toThrow();

    const chunks: Uint8Array[] = [];
    let reset = false;
    await sftpApi.streamDownload("edge", "transfer_test01", "/remote/file.bin", false, 3, {
      revision: '"old"', onReset: () => { reset = true; }, onChunk: (chunk) => { chunks.push(chunk); },
    });
    expect(reset).toBe(true);
    expect(new TextDecoder().decode(chunks[0])).toBe("abcxyz");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
