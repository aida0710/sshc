import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEntry, RemoteTextFile } from "./api";
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

const entry: RemoteEntry = { name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 6, mode: "0644", modifiedAt: "", revision: "meta" };

function textFile(contents: string, revision: string): RemoteTextFile {
  return { entry, contents, revision };
}

function Harness({ source, onSaved = async () => undefined }: {
  source: SFTPTextSource;
  onSaved?: (alias: string, saved: RemoteTextFile) => Promise<void>;
}) {
  const editor = useSFTPTextEditor({ source, onProblem: () => undefined, onSaved });
  return (
    <>
      <button type="button" onClick={() => void editor.open("edge", entry)}>Open notes</button>
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

describe("SFTP text editor", () => {
  it("edits the saved revision after a save even when refreshing the listing fails", async () => {
    const source = {
      readText: vi.fn(async () => textFile("hello\n", "rev-1")),
      saveText: vi.fn(async (_alias: string, _path: string, contents: string) => textFile(contents, `rev-${source.saveText.mock.calls.length + 1}`)),
    };
    // The pane reports a failed refresh in its own banner; the editor is not told.
    const onSaved = vi.fn(async () => undefined);
    render(<Harness source={source} onSaved={onSaved} />);

    const contents = await openAndEdit("one");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.queryByText("Unsaved")).not.toBeInTheDocument());
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\none", "rev-1");
    expect(onSaved).toHaveBeenCalledWith("edge", textFile("hello\none", "rev-2"));

    await userEvent.type(contents, "two");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(source.saveText).toHaveBeenCalledTimes(2));
    expect(source.saveText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt", "hello\nonetwo", "rev-2");
  });

  it("keeps the contents read-only while a save is in flight", async () => {
    let finishSave: (saved: RemoteTextFile) => void = () => undefined;
    const source = {
      readText: vi.fn(async () => textFile("hello\n", "rev-1")),
      saveText: vi.fn(() => new Promise<RemoteTextFile>((resolve) => { finishSave = resolve; })),
    };
    render(<Harness source={source} />);

    const contents = await openAndEdit("x");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(contents).toHaveAttribute("readonly");
    finishSave(textFile("hello\nx", "rev-2"));
    await waitFor(() => expect(contents).not.toHaveAttribute("readonly"));
  });

  it("shows a conflict inside the editor and reloads the remote file after confirming", async () => {
    const source = {
      readText: vi.fn()
        .mockResolvedValueOnce(textFile("hello\n", "rev-1"))
        .mockResolvedValueOnce(textFile("changed elsewhere\n", "rev-9")),
      saveText: vi.fn(async () => {
        throw new ApiError("sftp_conflict", 409, null);
      }),
    };
    render(<Harness source={source} />);

    const contents = await openAndEdit("mine");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    const editor = screen.getByRole("dialog", { name: "/remote/notes.txt" });
    const alert = await within(editor).findByRole("alert");
    expect(alert).toHaveTextContent("The remote file changed. Reload it before saving again.");

    await userEvent.click(within(alert).getByRole("button", { name: "Reload from remote" }));
    const confirmation = await screen.findByRole("dialog", { name: "Reload from remote?" });
    await userEvent.click(within(confirmation).getByRole("button", { name: "Discard and reload" }));
    await waitFor(() => expect(contents).toHaveValue("changed elsewhere\n"));
    expect(screen.queryByText("Unsaved")).not.toBeInTheDocument();
    expect(within(editor).queryByRole("alert")).not.toBeInTheDocument();
    expect(source.readText).toHaveBeenLastCalledWith("edge", "/remote/notes.txt");
  });

  it("shows a failed save inside the editor", async () => {
    const source = {
      readText: vi.fn(async () => textFile("hello\n", "rev-1")),
      saveText: vi.fn(async () => {
        throw new ApiError("sftp_permission_denied", 403, null);
      }),
    };
    render(<Harness source={source} />);

    await openAndEdit("mine");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    const alert = await within(screen.getByRole("dialog", { name: "/remote/notes.txt" })).findByRole("alert");
    expect(alert).not.toHaveTextContent("Reload from remote");
    expect(screen.getByText("Unsaved")).toBeInTheDocument();
  });

  it("asks before Escape closes an editor with unsaved changes", async () => {
    const source = { readText: vi.fn(async () => textFile("hello\n", "rev-1")), saveText: vi.fn() };
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
    const source = { readText: vi.fn(async () => textFile("hello\n", "rev-1")), saveText: vi.fn() };
    render(<Harness source={source} />);

    await userEvent.click(screen.getByRole("button", { name: "Open notes" }));
    await screen.findByRole("textbox", { name: "Remote file contents" });
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "/remote/notes.txt" })).not.toBeInTheDocument());
    expect(screen.queryByRole("dialog", { name: "Close without saving?" })).not.toBeInTheDocument();
  });
});
