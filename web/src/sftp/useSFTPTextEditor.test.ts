import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { notesEntry, notesFile, pendingRead, readsThenRereads, saveConflict } from "../testing/sftpTextFiles";
import { useSFTPTextEditor, type SFTPTextSource } from "./useSFTPTextEditor";

it("saves once when the shortcut is repeated and clears saved status on the next edit", async () => {
  let finish!: (file: ReturnType<typeof notesFile>) => void;
  const source = { readText: vi.fn(async () => notesFile("hello", "r1")), saveText: vi.fn(() => new Promise<ReturnType<typeof notesFile>>((resolve) => { finish = resolve; })) };
  const { result } = renderHook(() => useSFTPTextEditor({ source, onProblem: () => undefined, onSaved: async () => undefined }));
  await act(() => result.current.open("edge", notesEntry));
  act(() => result.current.setContents("edited"));
  let writing!: Promise<void>;
  act(() => { writing = result.current.save(); void result.current.save(); });
  expect(source.saveText).toHaveBeenCalledTimes(1);
  expect(result.current.saving).toBe(true);
  await act(async () => { finish(notesFile("edited", "r2")); await writing; });
  expect(result.current.saving).toBe(false);
  expect(result.current.saved).toBe(true);
  act(() => result.current.setContents("edited again"));
  expect(result.current.saved).toBe(false);
});

// Opens notes.txt at rev-1, edits it, and has the save refused because the
// remote file changed.
async function editorInConflict(source: SFTPTextSource) {
  const rendered = renderHook(() => useSFTPTextEditor({ source, onProblem: () => undefined, onSaved: async () => undefined }));
  await act(() => rendered.result.current.open("edge", notesEntry));
  act(() => rendered.result.current.setContents("hello\nmine"));
  await act(() => rendered.result.current.save());
  return rendered;
}

// Brings the editor into a conflict and presses Overwrite, with the read of
// the remote revision held. `answerRead` answers it with rev-9 and waits until
// the editor has taken the answer, so a test can act on the confirmation
// before or after the revision arrives.
async function startOverwriteRead() {
  const read = pendingRead();
  const source = { readText: readsThenRereads(() => read.promise), saveText: vi.fn(async () => { throw saveConflict; }) };
  const { result } = await editorInConflict(source);
  let reading: Promise<void> = Promise.resolve();
  act(() => {
    reading = result.current.requestOverwrite();
  });
  const answerRead = () => act(async () => {
    read.answer(notesFile("changed elsewhere\n", "rev-9"));
    await reading;
  });
  return { result, source, answerRead };
}

describe("useSFTPTextEditor", () => {
  it("opens a search match at its line only while its metadata revision still matches", async () => {
    const file = notesFile("first\nsecond\nmatching\n", "content-revision");
    const source = { readText: vi.fn(async () => file), saveText: vi.fn() };
    const onProblem = vi.fn();
    const { result } = renderHook(() => useSFTPTextEditor({ source, onProblem, onSaved: async () => undefined }));
    await act(() => result.current.open("edge", file.entry, { line: 3, expectedRevision: file.entry.revision }));
    expect(result.current.initialLine).toBe(3);
    expect(result.current.opened).toEqual(file);
    act(() => result.current.close());
    await act(() => result.current.open("edge", file.entry, { line: 3, expectedRevision: "old-metadata" }));
    expect(result.current.opened).toBeNull();
    expect(onProblem).toHaveBeenLastCalledWith("The file changed after the search. Search again before opening this match.");
  });
  it("offers to reload or overwrite after the save is refused as a conflict", async () => {
    const source = {
      readText: vi.fn(async () => notesFile("hello\n", "rev-1")),
      saveText: vi.fn(async () => { throw saveConflict; }),
    };
    const { result } = await editorInConflict(source);

    expect(result.current.problem).toEqual({
      message: "Could not save. The remote file changed after it was opened in the editor. Reload it from the remote, or overwrite it.",
      conflict: true,
    });
    expect(result.current.dirty).toBe(true);
  });

  it("overwrites against the revision read when the confirmation opened, without reading the remote file again", async () => {
    const source = {
      readText: readsThenRereads(),
      saveText: vi.fn()
        .mockRejectedValueOnce(saveConflict)
        .mockImplementationOnce(async (_alias: string, _path: string, contents: string) => notesFile(contents, "rev-10")),
    };
    const { result } = await editorInConflict(source);

    await act(() => result.current.requestOverwrite());
    expect(result.current.confirmingOverwrite).toBe(true);
    expect(result.current.readingOverwriteRevision).toBe(false);
    expect(result.current.contents).toBe("hello\nmine");

    await act(() => result.current.overwrite());
    expect(source.readText).toHaveBeenCalledTimes(2);
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\nmine", "rev-9");
    expect(result.current.confirmingOverwrite).toBe(false);
    expect(result.current.opened).toEqual(notesFile("hello\nmine", "rev-10"));
    expect(result.current.dirty).toBe(false);
    expect(result.current.problem).toBeNull();
  });

  it("keeps reload and overwrite offered when the confirmed overwrite fails for a reason other than a conflict", async () => {
    const source = {
      readText: readsThenRereads(),
      saveText: vi.fn()
        .mockRejectedValueOnce(saveConflict)
        .mockRejectedValueOnce(new ApiError("sftp_failed", 502, null)),
    };
    const { result } = await editorInConflict(source);

    await act(() => result.current.requestOverwrite());
    await act(() => result.current.overwrite());

    expect(result.current.problem).toEqual({ message: "The SFTP operation failed.", conflict: true });
    expect(result.current.dirty).toBe(true);
    expect(result.current.opened?.revision).toBe("rev-1");
  });

  it("writes nothing while the remote revision for the overwrite is still being read", async () => {
    const { result, source, answerRead } = await startOverwriteRead();

    expect(result.current.confirmingOverwrite).toBe(true);
    expect(result.current.readingOverwriteRevision).toBe(true);
    await act(() => result.current.overwrite());
    expect(source.saveText).toHaveBeenCalledTimes(1);

    await answerRead();
    expect(result.current.readingOverwriteRevision).toBe(false);
  });

  it("closes the confirmation and keeps the conflict when the remote file cannot be read for an overwrite", async () => {
    const source = {
      readText: readsThenRereads(async () => { throw new ApiError("sftp_not_found", 404, null); }),
      saveText: vi.fn(async () => { throw saveConflict; }),
    };
    const { result } = await editorInConflict(source);

    await act(() => result.current.requestOverwrite());

    expect(result.current.confirmingOverwrite).toBe(false);
    expect(result.current.problem).toEqual({
      message: "The file or folder was not found. It may have been moved or deleted.",
      conflict: true,
    });
    expect(source.saveText).toHaveBeenCalledTimes(1);
  });

  it("drops the revision read for an overwrite that was cancelled before the read answered", async () => {
    const { result, source, answerRead } = await startOverwriteRead();

    act(() => result.current.cancelOverwrite());
    await answerRead();

    expect(result.current.confirmingOverwrite).toBe(false);
    expect(source.saveText).toHaveBeenCalledTimes(1);
  });

  it("drops the revision read for an overwrite when the editor is closed before it arrives", async () => {
    const { result, source, answerRead } = await startOverwriteRead();

    act(() => result.current.close());
    await answerRead();

    expect(result.current.opened).toBeNull();
    expect(result.current.confirmingOverwrite).toBe(false);
    expect(source.saveText).toHaveBeenCalledTimes(1);
  });
});
