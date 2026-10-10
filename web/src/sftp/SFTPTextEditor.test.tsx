import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { notesEntry, notesFile, pendingRead, readsThenRereads, saveConflict } from "../testing/sftpTextFiles";
import type { RemoteTextFile } from "./api";
import { SFTPTextEditor } from "./SFTPTextEditor";
import { useSFTPTextEditor, type SFTPTextSource } from "./useSFTPTextEditor";

vi.mock("./MonacoEditor", () => ({
  MonacoEditor: ({ value, onChange, readOnly }: { value: string; onChange: (value: string) => void; readOnly: boolean }) => (
    <textarea
      aria-label="Remote file contents"
      value={value}
      readOnly={readOnly}
      onChange={(event) => onChange(event.currentTarget.value)}
    />
  ),
}));

function Harness({ source, onSaved = async () => undefined }: {
  source: SFTPTextSource;
  onSaved?: (alias: string, saved: RemoteTextFile) => Promise<void>;
}) {
  const editor = useSFTPTextEditor({ source, onProblem: () => undefined, onSaved });
  return (
    <>
      <button type="button" onClick={() => void editor.open("edge", notesEntry)}>Open notes</button>
      <SFTPTextEditor editor={editor} />
    </>
  );
}

async function openAndEdit(typed: string): Promise<HTMLElement> {
  await userEvent.click(screen.getByRole("button", { name: "Open notes" }));
  const contents = await screen.findByRole("textbox", { name: "Remote file contents" });
  await userEvent.type(contents, typed);
  await screen.findByText("Unsaved");
  return contents;
}

// Opens notes.txt, types "mine" and presses Save, which the source refuses.
// Returns the editor and the alert it shows the refusal in.
async function editUntilSaveFails(): Promise<{ contents: HTMLElement; editor: HTMLElement; alert: HTMLElement }> {
  const contents = await openAndEdit("mine");
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  const editor = screen.getByRole("dialog", { name: "/remote/notes.txt" });
  const alert = await within(editor).findByRole("alert");
  return { contents, editor, alert };
}

// Has the save refused as a conflict and presses Overwrite in the editor's
// alert. The confirmation it returns may still be reading the remote file.
async function openOverwriteConfirmation(): Promise<{ contents: HTMLElement; editor: HTMLElement; confirmation: HTMLElement }> {
  const { contents, editor, alert } = await editUntilSaveFails();
  await userEvent.click(within(alert).getByRole("button", { name: "Overwrite" }));
  const confirmation = await screen.findByRole("dialog", { name: "Overwrite the remote file?" });
  return { contents, editor, confirmation };
}

// The confirmation reads the remote file before its Overwrite button can be pressed.
async function confirmOverwrite(confirmation: HTMLElement) {
  const overwrite = within(confirmation).getByRole("button", { name: "Overwrite" });
  await waitFor(() => expect(overwrite).toBeEnabled());
  await userEvent.click(overwrite);
}

describe("SFTP text editor", () => {
  it("edits the saved revision after a save even when refreshing the listing fails", async () => {
    const source = {
      readText: vi.fn(async () => notesFile("hello\n", "rev-1")),
      saveText: vi.fn(async (_alias: string, _path: string, contents: string) => notesFile(contents, `rev-${source.saveText.mock.calls.length + 1}`)),
    };
    // The pane reports a failed refresh in its own banner; the editor is not told.
    const onSaved = vi.fn(async () => undefined);
    render(<Harness source={source} onSaved={onSaved} />);

    const contents = await openAndEdit("one");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.queryByText("Unsaved")).not.toBeInTheDocument());
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\none", "rev-1");
    expect(onSaved).toHaveBeenCalledWith("edge", notesFile("hello\none", "rev-2"));

    await userEvent.type(contents, "two");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(source.saveText).toHaveBeenCalledTimes(2));
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\nonetwo", "rev-2");
  });

  it("keeps the contents read-only while a save is in flight", async () => {
    let finishSave: (saved: RemoteTextFile) => void = () => undefined;
    const source = {
      readText: vi.fn(async () => notesFile("hello\n", "rev-1")),
      saveText: vi.fn(() => new Promise<RemoteTextFile>((resolve) => { finishSave = resolve; })),
    };
    render(<Harness source={source} />);

    const contents = await openAndEdit("x");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(contents).toHaveAttribute("readonly");
    finishSave(notesFile("hello\nx", "rev-2"));
    await waitFor(() => expect(contents).not.toHaveAttribute("readonly"));
  });

  it("shows a conflict inside the editor and reloads the remote file after confirming", async () => {
    const source = { readText: readsThenRereads(), saveText: vi.fn(async () => { throw saveConflict; }) };
    render(<Harness source={source} />);

    const { contents, editor, alert } = await editUntilSaveFails();
    expect(alert).toHaveTextContent("Could not save. The remote file changed after it was opened in the editor.");

    await userEvent.click(within(alert).getByRole("button", { name: "Reload from remote" }));
    const confirmation = await screen.findByRole("dialog", { name: "Reload from remote?" });
    await userEvent.click(within(confirmation).getByRole("button", { name: "Discard and reload" }));
    await waitFor(() => expect(contents).toHaveValue("changed elsewhere\n"));
    expect(screen.queryByText("Unsaved")).not.toBeInTheDocument();
    expect(within(editor).queryByRole("alert")).not.toBeInTheDocument();
    expect(source.readText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt");
  });

  it("overwrites the remote file with the editor's contents after confirming", async () => {
    const source = {
      readText: readsThenRereads(),
      saveText: vi.fn()
        .mockRejectedValueOnce(saveConflict)
        .mockImplementationOnce(async (_alias: string, _path: string, contents: string) => notesFile(contents, "rev-10")),
    };
    const onSaved = vi.fn(async () => undefined);
    render(<Harness source={source} onSaved={onSaved} />);

    const { contents, editor, confirmation } = await openOverwriteConfirmation();
    expect(confirmation).toHaveTextContent("Overwriting replaces it with the editor's contents, and the changes made on the remote are lost.");
    expect(source.saveText).toHaveBeenCalledTimes(1);

    await confirmOverwrite(confirmation);
    await waitFor(() => expect(screen.queryByText("Unsaved")).not.toBeInTheDocument());
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\nmine", "rev-9");
    expect(onSaved).toHaveBeenCalledWith("edge", notesFile("hello\nmine", "rev-10"));
    expect(contents).toHaveValue("hello\nmine");
    expect(within(editor).queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Overwrite the remote file?" })).not.toBeInTheDocument();
  });

  it("holds the confirmation's Overwrite button and says the remote file is being read until it is, with focus on Cancel", async () => {
    const read = pendingRead();
    const source = { readText: readsThenRereads(() => read.promise), saveText: vi.fn(async () => { throw saveConflict; }) };
    render(<Harness source={source} />);

    const { confirmation } = await openOverwriteConfirmation();
    const overwrite = within(confirmation).getByRole("button", { name: "Overwrite" });
    expect(overwrite).toBeDisabled();
    expect(within(confirmation).getByRole("status")).toHaveTextContent("Reading the remote file…");
    expect(within(confirmation).getByRole("button", { name: "Cancel" })).toHaveFocus();
    // Opening its own confirmation does not ask whether to close the editor.
    expect(screen.queryByRole("dialog", { name: "Close without saving?" })).not.toBeInTheDocument();

    read.answer(notesFile("changed elsewhere\n", "rev-9"));
    await waitFor(() => expect(overwrite).toBeEnabled());
    expect(within(confirmation).queryByRole("status")).not.toBeInTheDocument();
    expect(within(confirmation).getByRole("button", { name: "Cancel" })).toHaveFocus();
  });

  it("writes nothing when the overwrite is cancelled", async () => {
    const source = { readText: readsThenRereads(), saveText: vi.fn(async () => { throw saveConflict; }) };
    render(<Harness source={source} />);

    const { contents, editor, confirmation } = await openOverwriteConfirmation();
    await userEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Overwrite the remote file?" })).not.toBeInTheDocument());
    expect(source.saveText).toHaveBeenCalledTimes(1);
    expect(contents).toHaveValue("hello\nmine");
    expect(screen.getByText("Unsaved")).toBeInTheDocument();
    expect(within(editor).getByRole("alert")).toHaveTextContent("Could not save. The remote file changed after it was opened in the editor.");
  });

  it("refuses the overwrite as a conflict again when the remote file changes while it is confirmed", async () => {
    // The sshc engine sees a remote file that is no longer rev-9 by the time
    // the confirmed overwrite arrives.
    const source = { readText: readsThenRereads(), saveText: vi.fn(async () => { throw saveConflict; }) };
    render(<Harness source={source} />);

    const { editor, confirmation } = await openOverwriteConfirmation();
    await confirmOverwrite(confirmation);

    await waitFor(() => expect(within(editor).getByRole("alert")).toHaveTextContent(
      "Could not overwrite. The remote file changed again while the overwrite was being confirmed.",
    ));
    expect(source.saveText).toHaveBeenCalledTimes(2);
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\nmine", "rev-9");
    expect(screen.getByText("Unsaved")).toBeInTheDocument();
    const alert = within(editor).getByRole("alert");
    expect(within(alert).getByRole("button", { name: "Overwrite" })).toBeEnabled();
    expect(within(alert).getByRole("button", { name: "Reload from remote" })).toBeEnabled();
  });

  it("shows a failed save inside the editor", async () => {
    const source = {
      readText: vi.fn(async () => notesFile("hello\n", "rev-1")),
      saveText: vi.fn(async () => {
        throw new ApiError("sftp_permission_denied", 403, null);
      }),
    };
    render(<Harness source={source} />);

    const { alert } = await editUntilSaveFails();
    expect(within(alert).queryByRole("button", { name: "Reload from remote" })).not.toBeInTheDocument();
    expect(within(alert).queryByRole("button", { name: "Overwrite" })).not.toBeInTheDocument();
    expect(screen.getByText("Unsaved")).toBeInTheDocument();
  });

  it("asks before Escape closes an editor with unsaved changes", async () => {
    const source = { readText: vi.fn(async () => notesFile("hello\n", "rev-1")), saveText: vi.fn() };
    render(<Harness source={source} />);

    await openAndEdit("mine");
    await userEvent.keyboard("{Escape}");
    let confirmation = await screen.findByRole("dialog", { name: "Close without saving?" });
    expect(confirmation).toHaveTextContent("/remote/notes.txt has changes that are not written to the server.");
    await userEvent.click(within(confirmation).getByRole("button", { name: "Keep editing" }));
    expect(screen.getByRole("dialog", { name: "/remote/notes.txt" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Remote file contents" })).toHaveValue("hello\nmine");

    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    confirmation = await screen.findByRole("dialog", { name: "Close without saving?" });
    await userEvent.click(within(confirmation).getByRole("button", { name: "Discard and close" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "/remote/notes.txt" })).not.toBeInTheDocument());
    expect(source.saveText).not.toHaveBeenCalled();
  });

  it("closes an editor without changes at once", async () => {
    const source = { readText: vi.fn(async () => notesFile("hello\n", "rev-1")), saveText: vi.fn() };
    render(<Harness source={source} />);

    await userEvent.click(screen.getByRole("button", { name: "Open notes" }));
    await screen.findByRole("textbox", { name: "Remote file contents" });
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "/remote/notes.txt" })).not.toBeInTheDocument());
    expect(screen.queryByRole("dialog", { name: "Close without saving?" })).not.toBeInTheDocument();
  });
});
