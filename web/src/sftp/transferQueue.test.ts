import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { SFTPTransferQueue } from "./transferQueue";

function fakeAPI() {
  let offset = 0;
  return {
    startUpload: vi.fn(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset, size, expectedRevision: "absent" })),
    appendUpload: vi.fn(async (_alias: string, id: string, path: string, at: number, size: number, chunk: Blob) => {
      expect(at).toBe(offset);
      offset += chunk.size;
      return { id, path, offset, size, expectedRevision: "" };
    }),
    completeUpload: vi.fn(async () => undefined),
    cancelUpload: vi.fn(async () => undefined),
    setOffset(value: number) { offset = value; },
  };
}

describe("SFTPTransferQueue", () => {
  beforeEach(() => localStorage.clear());

  it("uploads large files in bounded chunks and completes them", async () => {
    const api = fakeAPI();
    const queue = new SFTPTransferQueue(api);
    const file = new File([new Uint8Array((2 << 20) + 17)], "large.bin", { lastModified: 123 });
    queue.add([{ alias: "edge", remotePath: "/remote/large.bin", localName: "large.bin", file }]);

    await waitFor(() => expect(queue.getSnapshot()[0]?.status).toBe("done"));
    expect(api.appendUpload).toHaveBeenCalledTimes(3);
    expect(api.appendUpload.mock.calls.map((call) => call[5].size)).toEqual([1 << 20, 1 << 20, 17]);
    expect(api.completeUpload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/large.bin", file.size, "absent");
  });

  it("restores metadata as reattach-required and resumes at the remote offset", async () => {
    localStorage.setItem("sshc.sftp.transfer-queue.v1", JSON.stringify([{
      id: "upload_restore1", alias: "edge", remotePath: "/remote/file.bin", localName: "file.bin",
      size: 8, lastModified: 456, offset: 3, status: "uploading", expectedRevision: "absent", overwrite: false,
    }]));
    const api = fakeAPI();
    api.setOffset(3);
    const queue = new SFTPTransferQueue(api);
    expect(queue.getSnapshot()[0]?.status).toBe("reattach");

    const file = new File(["abcdefgh"], "file.bin", { lastModified: 456 });
    queue.add([{ alias: "edge", remotePath: "/remote/file.bin", localName: "file.bin", file }]);

    await waitFor(() => expect(queue.getSnapshot()[0]?.status).toBe("done"));
    expect(api.appendUpload).toHaveBeenCalledTimes(1);
    expect(api.appendUpload.mock.calls[0]?.[3]).toBe(3);
  });

  it("waits for explicit overwrite confirmation", async () => {
    const api = fakeAPI();
    api.startUpload.mockRejectedValueOnce(new ApiError("sftp_exists", 409, null));
    const queue = new SFTPTransferQueue(api);
    const file = new File(["new"], "existing.txt", { lastModified: 789 });
    queue.add([{ alias: "edge", remotePath: "/remote/existing.txt", localName: "existing.txt", file }]);
    await waitFor(() => expect(queue.getSnapshot()[0]?.status).toBe("needs_overwrite"));

    const id = queue.getSnapshot()[0]?.id;
    expect(id).toBeDefined();
    queue.overwrite(id!);
    await waitFor(() => expect(queue.getSnapshot()[0]?.status).toBe("done"));
    expect(api.startUpload).toHaveBeenLastCalledWith("edge", id, "/remote/existing.txt", 3, true, "");
  });
});
