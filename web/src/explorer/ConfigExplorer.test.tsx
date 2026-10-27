import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConfigExplorer } from "./ConfigExplorer";
import { ApiError } from "../api/client";
import { configApi } from "../api/config";
import { captureNavigationBlocker } from "../testing/navigationBlocker";
import { ToastProvider } from "../ui/ToastProvider";

vi.mock("../api/config", async () => {
  const actual = await vi.importActual<typeof import("../api/config")>("../api/config");
  return { ...actual, configApi: { overview: vi.fn(), file: vi.fn(), preview: vi.fn(), save: vi.fn() } };
});

const overview = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [
    {
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      editable: true,
      loads: 1,
      includes: [{
        line: 2,
        pattern: "conf.d/*.conf",
        matches: [{ path: "conf.d/10-home.conf", absolute: "/home/tester/.ssh/conf.d/10-home.conf" }],
      }, {
        line: 3,
        pattern: "connections/dubguild/high-performance-computing/*.conf",
        condition: "Host mado-office-with-a-long-condition",
        matches: [{
          path: "connections/dubguild/high-performance-computing/mado-office.conf",
          absolute: "/home/tester/.ssh/connections/dubguild/high-performance-computing/mado-office.conf",
        }],
      }],
    },
    { file: { path: "conf.d/10-home.conf", absolute: "/home/tester/.ssh/conf.d/10-home.conf" }, editable: true, loads: 1 },
    { file: { absolute: "/etc/ssh/ssh_config", external: true }, editable: false, loads: 1 },
  ],
  hosts: [],
  metadata: { schemaVersion: 1 },
  diagnostics: [{ severity: "warning", code: "include_no_match", path: "config", line: 2, detail: "conf.d/*.conf" }],
  notices: [],
};

// chooseFileAction opens the "…" menu of the open file and chooses one of its actions.
async function chooseFileAction(user: ReturnType<typeof userEvent.setup>, path: string, action: string) {
  await user.click(await screen.findByRole("button", { name: `Actions for ${path}` }));
  await user.click(screen.getByRole("menuitem", { name: action }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(configApi.overview).mockResolvedValue(overview as never);
  vi.mocked(configApi.file).mockImplementation(async (path) => ({
    file: { path, absolute: `/home/tester/.ssh/${path}` },
    contents: path === "config" ? "Include conf.d/*.conf\n" : "Host nas\n\tUser aida\n",
    digest: "digest",
    editable: true,
    exists: true,
  }));
});

describe("ConfigExplorer", () => {
  it("reports the pending file load while another file opens", async () => {
    const user = userEvent.setup();
    let complete: (() => void) | undefined;
    render(<ConfigExplorer />);
    await screen.findByLabelText(/File text.*config/);
    vi.mocked(configApi.file).mockImplementationOnce((path) => new Promise((resolve) => {
      complete = () => resolve({
        file: { path, absolute: `/home/tester/.ssh/${path}` },
        contents: "Host next\n", digest: "next", editable: true, exists: true,
      });
    }));
    await user.click(screen.getByRole("button", { name: "conf.d/10-home.conf" }));
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByLabelText(/File text/)).not.toBeInTheDocument();
    complete?.();
    expect(await screen.findByLabelText(/File text.*conf\.d/)).toHaveValue("Host next\n");
  });

  it("opens the entry file by default instead of leaving the editor empty", async () => {
    render(<ConfigExplorer />);

    await waitFor(() => expect(configApi.file).toHaveBeenCalledWith("config"));
    expect(await screen.findByLabelText(/File text.*config/)).toHaveValue("Include conf.d/*.conf\n");
  });

  it("keeps the latest file selection when the automatic entry load returns late", async () => {
    let finishEntry: ((value: unknown) => void) | undefined;
    vi.mocked(configApi.file).mockImplementation((path) => {
      if (path === "config") {
        return new Promise((resolve) => {
          finishEntry = resolve;
        }) as never;
      }
      return Promise.resolve({
        file: { path, absolute: `/home/tester/.ssh/${path}` },
        contents: "Host nas\n\tUser aida\n",
        digest: "digest",
        editable: true,
        exists: true,
      });
    });
    const user = userEvent.setup();
    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    expect(await screen.findByLabelText(/File text.*conf\.d\/10-home\.conf/)).toBeInTheDocument();

    finishEntry?.({
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      contents: "Include conf.d/*.conf\n",
      digest: "entry-digest",
      editable: true,
      exists: true,
    });
    await waitFor(() =>
      expect(screen.getByLabelText(/File text.*conf\.d\/10-home\.conf/)).toBeInTheDocument(),
    );
    expect(screen.queryByLabelText(/File text — config\./)).not.toBeInTheDocument();
  });

  it("shows the include hierarchy, the reference graph and the diagnostics", async () => {
    render(<ConfigExplorer />);

    expect(await screen.findByRole("button", { name: "config" })).toBeInTheDocument();
    expect(screen.getByText("conf.d/*.conf")).toBeInTheDocument();
    expect(screen.getByText(/include_no_match/)).toBeInTheDocument();
  });

  it("sums up the files and diagnostics beside the heading", async () => {
    render(<ConfigExplorer />);

    expect(await screen.findByText("Loaded files 3 · Editable 2 · Diagnostics 1")).toBeInTheDocument();
  });

  it("indents a file under the Include that reads it, and notes only what is unusual about a file", async () => {
    render(<ConfigExplorer />);

    const entry = (await screen.findByRole("button", { name: "config" })).closest("li");
    const included = screen.getByRole("button", { name: "conf.d/10-home.conf" }).closest("li");
    expect(entry).toHaveStyle({ marginLeft: "0px" });
    expect(included).toHaveStyle({ marginLeft: "14px" });
    expect(within(screen.getByRole("region", { name: "Include hierarchy" })).queryByText("editable")).not.toBeInTheDocument();
  });

  it("wraps long Include paths and conditions inside the hierarchy card", async () => {
    render(<ConfigExplorer />);

    const pattern = await screen.findByText("connections/dubguild/high-performance-computing/*.conf");
    const condition = screen.getByText("inside Host mado-office-with-a-long-condition");

    expect(pattern).toHaveClass("block", "break-all");
    expect(pattern.closest("div")).toHaveClass("min-w-0", "overflow-hidden");
    expect(condition).toHaveClass("block", "break-words");
  });

  it("marks a file outside ~/.ssh as read only", async () => {
    render(<ConfigExplorer />);

    expect(await screen.findByText("/etc/ssh/ssh_config")).toBeInTheDocument();
    expect(screen.getByText(/outside ~\/\.ssh/i)).toBeInTheDocument();
  });

  it("marks which file the editor is showing", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);

    const target = await screen.findByRole("button", { name: "conf.d/10-home.conf" });
    expect(target).toHaveAttribute("aria-current", "false");

    await user.click(target);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "conf.d/10-home.conf" })).toHaveAttribute("aria-current", "true"),
    );
    expect(screen.getByRole("button", { name: "config" })).toHaveAttribute("aria-current", "false");
  });

  it("says when the draft no longer matches what was read", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    const editor = await screen.findByLabelText(/File text/);
    expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();

    await user.type(editor, "\tPort 2222\n");

    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  });

  it("offers no file creation until a path is typed", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "Create file" }));
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();

    await user.type(screen.getByLabelText("New file path"), "conf.d/30-lab.conf");

    expect(screen.getByRole("button", { name: "Create" })).toBeEnabled();
  });

  it("creates a folder typed from ~/.ssh in ~/.ssh instead of nesting another ~ inside it", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1", written: ["conf.d"], preview: { operation: "config.directory_create", diffs: [] },
    });
    render(<ConfigExplorer />);

    await chooseFileAction(user, "config", "Create directory");
    await user.type(screen.getByLabelText("Directory path"), "~/.ssh/conf.d");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith({ kind: "directory_create", path: "conf.d" }));
  });

  it("explains a path outside ~/.ssh under the field and sends nothing", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);

    await chooseFileAction(user, "config", "Create directory");
    const field = screen.getByLabelText("Directory path");
    await user.type(field, "~/notes");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Paths outside ~/.ssh cannot be used.");
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(configApi.save).not.toHaveBeenCalled();
    await user.type(field, "x");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("renames a file to a path typed from ~/.ssh and refuses one outside it", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1", written: ["conf.d/11-home.conf"], preview: { operation: "config.file_rename", diffs: [] },
    });
    render(<ConfigExplorer />);
    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    await chooseFileAction(user, "conf.d/10-home.conf", "Rename file");
    const field = screen.getByLabelText("New path");

    await user.type(field, "~/other.conf");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Paths outside ~/.ssh cannot be used.");
    expect(configApi.save).not.toHaveBeenCalled();

    await user.clear(field);
    await user.type(field, "~/.ssh/conf.d/11-home.conf");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith(expect.objectContaining({
      kind: "file_rename", path: "conf.d/10-home.conf", destinationPath: "conf.d/11-home.conf",
    })));
  });

  it("edits a whole file and saves it with the loaded base", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1", written: ["conf.d/10-home.conf"], preview: { operation: "config.file_raw", diffs: [] },
    });

    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    const editor = await screen.findByLabelText(/File text/);
    await user.clear(editor);
    await user.type(editor, "Host nas\n\tUser root\n");
    await user.click(screen.getByRole("button", { name: "Save file" }));

    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith({
      kind: "file_raw",
      path: "conf.d/10-home.conf",
      base: "Host nas\n\tUser aida\n",
      raw: "Host nas\n\tUser root\n",
    }));
  });

  it("shows Save file working until the save is answered, and holds the other file actions meanwhile", async () => {
    const user = userEvent.setup();
    let answer: () => void = () => undefined;
    vi.mocked(configApi.save).mockImplementation(() => new Promise((resolve) => {
      answer = () => resolve({
        transactionId: "t1", written: ["conf.d/10-home.conf"], preview: { operation: "config.file_raw", diffs: [] },
      });
    }));
    render(<ToastProvider><ConfigExplorer /></ToastProvider>);

    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    await user.type(await screen.findByLabelText(/File text/), "\tPort 2222\n");
    const save = screen.getByRole("button", { name: "Save file" });
    await user.click(save);

    expect(save).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Preview" })).not.toHaveAttribute("aria-busy");
    await user.click(save);
    expect(configApi.save).toHaveBeenCalledTimes(1);
    answer();
    expect(await screen.findByText("Saved conf.d/10-home.conf.")).toBeInTheDocument();
  });

  it("says a directory was created, because the file list does not show directories", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1", written: ["conf.d/eu"], preview: { operation: "config.directory_create", diffs: [] },
    });
    render(<ToastProvider><ConfigExplorer /></ToastProvider>);

    await chooseFileAction(user, "config", "Create directory");
    await user.type(screen.getByLabelText("Directory path"), "conf.d/eu");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(await screen.findByText("Created the directory conf.d/eu.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Directory path")).not.toBeInTheDocument();
  });

  it("highlights a file row that can be opened when the pointer rests on it, but not one outside ~/.ssh", async () => {
    render(<ConfigExplorer />);

    const openable = (await screen.findByRole("button", { name: "conf.d/10-home.conf" })).closest("li");
    const external = screen.getByText("/etc/ssh/ssh_config").closest("li");
    expect(openable?.className).toContain("hover:bg-hover");
    expect(external?.className).not.toContain("hover:");
  });

  it("shows a refused delete once, inside the confirmation, and keeps the confirmation open", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockRejectedValue(
      new ApiError("config_conflict", 409, { code: "config_conflict", message: "changed on disk" }),
    );

    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    await screen.findByLabelText(/File text/);
    await chooseFileAction(user, "conf.d/10-home.conf", "Delete file");
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete it" }));

    const failure = await within(dialog).findByRole("alert");
    expect(failure).toHaveTextContent("The file changed outside this application. Nothing was written.");
    expect(screen.getAllByRole("alert")).toEqual([failure]);
    expect(screen.getByRole("dialog")).toBe(dialog);
  });

  it("keeps renaming and deleting the file out of reach while the draft is unsaved, but not the directory actions", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);
    await user.type(await screen.findByLabelText(/File text.*config/), "Host lab");

    await user.click(screen.getByRole("button", { name: "Actions for config" }));

    expect(screen.getByRole("menuitem", { name: "Rename file" })).toBeDisabled();
    expect(screen.getByRole("menuitem", { name: "Rename file" })).toHaveAttribute("title", expect.stringContaining("unsaved edits"));
    expect(screen.getByRole("menuitem", { name: "Delete file" })).toBeDisabled();
    expect(screen.getByRole("menuitem", { name: "Create directory" })).toBeEnabled();
    expect(screen.getByRole("menuitem", { name: "Delete directory" })).toBeEnabled();
  });

  it("keeps an edit begun after the rename field opened, and says to save it first instead of renaming", async () => {
    const user = userEvent.setup();
    render(<ToastProvider><ConfigExplorer /></ToastProvider>);
    await user.click(await screen.findByRole("button", { name: "conf.d/10-home.conf" }));
    const editor = await screen.findByLabelText(/File text.*10-home/);
    await chooseFileAction(user, "conf.d/10-home.conf", "Rename file");

    await user.type(editor, "  Port 2222");
    await user.type(screen.getByLabelText("New path"), "conf.d/11-home.conf");
    await user.click(screen.getByRole("button", { name: "Rename" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Save them or reopen the file before renaming");
    expect(configApi.save).not.toHaveBeenCalled();
    expect(editor).toHaveValue("Host nas\n\tUser aida\n  Port 2222");
  });

  it("deletes an empty directory typed after choosing Delete directory, and closes the field", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1", written: ["conf.d/eu"], preview: { operation: "config.directory_delete", diffs: [] },
    });
    render(<ConfigExplorer />);

    await chooseFileAction(user, "config", "Delete directory");
    await user.type(screen.getByLabelText("Directory path"), "conf.d/eu");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith({ kind: "directory_delete", path: "conf.d/eu" }));
    expect(screen.queryByLabelText("Directory path")).not.toBeInTheDocument();
  });

  it("closes the path field on Cancel without sending anything", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "Create file" }));
    await user.type(screen.getByLabelText("New file path"), "conf.d/30-lab.conf");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByLabelText("New file path")).not.toBeInTheDocument();
    expect(configApi.save).not.toHaveBeenCalled();
  });

  it("opens a target file and selects the targeted line", async () => {
    render(<ConfigExplorer target={{ path: "conf.d/10-home.conf", line: 2, request: 1 }} />);

    const editor = await screen.findByLabelText<HTMLTextAreaElement>(/File text/);

    expect(configApi.file).toHaveBeenCalledWith("conf.d/10-home.conf");
    await waitFor(() => expect(editor.selectionStart).toBe("Host nas\n".length));
    expect(editor.selectionEnd).toBe("Host nas\n\tUser aida".length);
    expect(editor).toHaveFocus();
    expect(screen.getByText(/conf\.d\/10-home\.conf.*line 2/)).toBeInTheDocument();
  });

  it("clears the handed target at once and still selects its line instead of opening the entry file", async () => {
    const onTargetHandled = vi.fn();
    const { rerender } = render(
      <ConfigExplorer target={{ path: "conf.d/10-home.conf", line: 2, request: 4 }} onTargetHandled={onTargetHandled} />,
    );
    expect(onTargetHandled).toHaveBeenCalledWith(4);
    rerender(<ConfigExplorer target={null} onTargetHandled={onTargetHandled} />);

    const editor = await screen.findByLabelText<HTMLTextAreaElement>(/File text.*conf\.d/);
    await waitFor(() => expect(editor.selectionStart).toBe("Host nas\n".length));
    expect(configApi.file).not.toHaveBeenCalledWith("config");
    expect(onTargetHandled).toHaveBeenCalledTimes(1);
  });

  it("creates a new configuration file inside ~/.ssh", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t2", written: ["conf.d/30-lab.conf"], preview: { operation: "config.file_raw", diffs: [] },
    });

    render(<ConfigExplorer />);

    await user.click(await screen.findByRole("button", { name: "Create file" }));
    await user.type(screen.getByLabelText("New file path"), "conf.d/30-lab.conf");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith({
      kind: "file_raw",
      path: "conf.d/30-lab.conf",
      base: "",
      raw: "# created by sshc\n",
    }));
  });

  it("asks before another file replaces an unsaved draft, and keeps the draft when editing continues", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);
    const editor = await screen.findByLabelText(/File text.*config/);
    await user.type(editor, "Host lab");

    await user.click(screen.getByRole("button", { name: "conf.d/10-home.conf" }));
    expect(screen.getByRole("dialog", { name: "Discard unsaved changes?" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Keep editing" }));

    expect(configApi.file).not.toHaveBeenCalledWith("conf.d/10-home.conf");
    expect(screen.getByLabelText(/File text.*config/)).toHaveValue("Include conf.d/*.conf\nHost lab");

    await user.click(screen.getByRole("button", { name: "conf.d/10-home.conf" }));
    await user.click(screen.getByRole("button", { name: "Discard" }));

    expect(await screen.findByLabelText(/File text.*conf\.d/)).toHaveValue("Host nas\n\tUser aida\n");
  });

  it("asks before a target in another file replaces an unsaved draft", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<ConfigExplorer />);
    await user.type(await screen.findByLabelText(/File text.*config/), "Host lab");

    rerender(<ConfigExplorer target={{ path: "conf.d/10-home.conf", line: 1, request: 1 }} />);

    expect(await screen.findByRole("dialog", { name: "Discard unsaved changes?" })).toBeVisible();
    expect(configApi.file).not.toHaveBeenCalledWith("conf.d/10-home.conf");
  });

  it("keeps the draft instead of reading the file again when the open file is chosen again", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);
    const editor = await screen.findByLabelText(/File text.*config/);
    await user.type(editor, "Host lab");

    await user.click(screen.getByRole("button", { name: "config" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(configApi.file).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText(/File text.*config/)).toHaveValue("Include conf.d/*.conf\nHost lab");
  });

  it("asks before creating a file, because the new file replaces the unsaved draft", async () => {
    const user = userEvent.setup();
    render(<ConfigExplorer />);
    await user.type(await screen.findByLabelText(/File text.*config/), "Host lab");
    await user.click(screen.getByRole("button", { name: "Create file" }));
    await user.type(screen.getByLabelText("New file path"), "conf.d/30-lab.conf");

    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(screen.getByRole("dialog", { name: "Discard unsaved changes?" })).toBeVisible();
    expect(configApi.save).not.toHaveBeenCalled();
  });

  it("stops leaving Config with an unsaved draft, but lets a move inside Config through", async () => {
    const user = userEvent.setup();
    const navigation = captureNavigationBlocker();
    const onNavigateLocation = vi.fn();
    render(
      <ConfigExplorer
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
      />,
    );
    await user.type(await screen.findByLabelText(/File text.*config/), "Host lab");

    expect(navigation.tryNavigate("/config")).toBe(true);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(navigation.tryNavigate("/keys")).toBe(false);
    await user.click(screen.getByRole("button", { name: "Discard" }));

    expect(onNavigateLocation).toHaveBeenCalledWith("/keys");
  });

  describe("on a phone", () => {
    beforeEach(() => {
      vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
        matches: true,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })));
    });

    it("shows the file chosen from the list in its place, and goes back to the list keeping the draft", async () => {
      const user = userEvent.setup();
      render(<ConfigExplorer />);
      expect(await screen.findByRole("button", { name: "Create file" })).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "config" }));
      expect(screen.queryByRole("button", { name: "Create file" })).toBeNull();
      await user.type(screen.getByLabelText(/File text.*config/), "Host lab");

      await user.click(screen.getByRole("button", { name: "All files" }));
      expect(screen.getByRole("button", { name: "Create file" })).toBeInTheDocument();
      expect(screen.getByLabelText(/File text.*config/)).toHaveValue("Include conf.d/*.conf\nHost lab");
      expect(configApi.file).toHaveBeenCalledTimes(1);
    });
  });
});
