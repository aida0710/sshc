import { vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEntry, RemoteTextFile } from "../sftp/api";
import type { SFTPTextSource } from "../sftp/useSFTPTextEditor";

// The file the SFTP text editor tests open.
export const notesEntry: RemoteEntry = {
  name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 6, mode: "0644", modifiedAt: "", revision: "meta",
};

// notesFile is notes.txt as the sshc engine reads it: its contents and the
// revision a save of it must name.
export function notesFile(contents: string, revision: string): RemoteTextFile {
  return { entry: notesEntry, contents, revision };
}

// The sshc engine's answer to a save whose expected revision is no longer the
// remote file's.
export const saveConflict = new ApiError("sftp_conflict", 409, null);

// A read of the remote file that answers only when the test says so.
export function pendingRead(): { promise: Promise<RemoteTextFile>; answer: (file: RemoteTextFile) => void } {
  let answer: (file: RemoteTextFile) => void = () => undefined;
  const promise = new Promise<RemoteTextFile>((resolve) => { answer = resolve; });
  return { promise, answer };
}

// readsThenRereads stands in for readText when notes.txt changes on the remote
// after the editor opened it: the first read finds "hello\n" at rev-1, and the
// next one runs `reread`, which by default finds "changed elsewhere\n" at rev-9.
export function readsThenRereads(
  reread: () => Promise<RemoteTextFile> = async () => notesFile("changed elsewhere\n", "rev-9"),
) {
  return vi.fn<SFTPTextSource["readText"]>()
    .mockResolvedValueOnce(notesFile("hello\n", "rev-1"))
    .mockImplementationOnce(reread);
}
