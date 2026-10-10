import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { notesEntry, notesFile } from "../testing/sftpTextFiles";
import { sftpApi } from "./api";
import { localHostAlias } from "./localHost";
import { useSFTPTextComparison } from "./useSFTPTextComparison";

vi.mock("./api", () => ({ sftpApi: { readText: vi.fn() } }));
beforeEach(() => vi.mocked(sftpApi.readText).mockReset());

it("pins both reads to listed revisions and compares local text against remote text", async () => {
  vi.mocked(sftpApi.readText).mockResolvedValueOnce(notesFile("left\n", "a")).mockResolvedValueOnce(notesFile("right\n", "b"));
  const { result } = renderHook(() => useSFTPTextComparison({ left: { alias: localHostAlias, entry: notesEntry }, right: { alias: "edge", entry: { ...notesEntry, revision: "r2" } } }));
  await waitFor(() => expect(result.current.comparison).toEqual({ original: "left\n", modified: "right\n" }));
  expect(sftpApi.readText).toHaveBeenCalledWith(localHostAlias, notesEntry.path, { expectedRevision: notesEntry.revision, signal: expect.any(AbortSignal) });
  expect(sftpApi.readText).toHaveBeenCalledWith("edge", notesEntry.path, { expectedRevision: "r2", signal: expect.any(AbortSignal) });
});

it("uses empty text for an absent side without creating or reading a missing file", async () => {
  vi.mocked(sftpApi.readText).mockResolvedValue(notesFile("only left", "a"));
  const { result } = renderHook(() => useSFTPTextComparison({ left: { alias: "edge", entry: notesEntry }, right: { alias: "other" } }));
  await waitFor(() => expect(result.current.comparison).toEqual({ original: "only left", modified: "" }));
  expect(sftpApi.readText).toHaveBeenCalledTimes(1);
});

it("reports changed files and aborts the other read", async () => {
  vi.mocked(sftpApi.readText).mockRejectedValueOnce(new ApiError("sftp_conflict", 409, null)).mockImplementationOnce(() => new Promise(() => undefined));
  const { result } = renderHook(() => useSFTPTextComparison({ left: { alias: "edge", entry: notesEntry }, right: { alias: "other", entry: notesEntry } }));
  await waitFor(() => expect(result.current.problem).not.toBe(""));
  expect(result.current.comparison).toBeNull();
  expect(vi.mocked(sftpApi.readText).mock.calls[1]?.[2]?.signal?.aborted).toBe(true);
});

it("retires reads on dismissal and ignores their late answers", async () => {
  let finish!: (file: ReturnType<typeof notesFile>) => void;
  vi.mocked(sftpApi.readText).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
  const { result, unmount } = renderHook(() => useSFTPTextComparison({ left: { alias: "edge", entry: notesEntry }, right: { alias: "other" } }));
  const signal = vi.mocked(sftpApi.readText).mock.calls[0]?.[2]?.signal;
  unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => finish(notesFile("late", "a")));
  expect(result.current.comparison).toBeNull();
});
