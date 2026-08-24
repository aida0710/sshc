import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { GroupsPanel, treeOrder } from "./GroupsPanel";
import { configApi } from "../api/config";

vi.mock("../api/config", async () => {
  const actual = await vi.importActual<typeof import("../api/config")>("../api/config");
  return {
    ...actual,
    configApi: {
      overview: vi.fn(),
      preview: vi.fn(),
      save: vi.fn(),
      renameGroup: vi.fn(),
      deleteGroup: vi.fn(),
    },
  };
});

const overview = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [],
  hosts: [
    {
      identity: { path: "connections/company/build.conf", alias: "build01" },
      file: { path: "connections/company/build.conf", absolute: "/home/tester/.ssh/connections/company/build.conf" },
      line: 1,
      patterns: ["build01"],
      editable: true,
      group: "company",
    },
  ],
  metadata: {
    schemaVersion: 2,
    groups: [{ name: "company", settings: [{ keyword: "ServerAliveInterval", values: ["30"] }] }],
    hosts: [{ identity: { path: "connections/company/build.conf", alias: "build01" } }],
  },
  groups: [],
  diagnostics: [],
  notices: [],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(configApi.overview).mockResolvedValue(overview as never);
  vi.mocked(configApi.preview).mockResolvedValue({
    operation: "config.groups",
    diffs: [{ path: "groups.sshc.conf", created: true, lines: [{ op: "insert", text: "Host build01", newLine: 1 }] }],
    effective: [{ alias: "build01", changes: [{ keyword: "Port", before: [], after: ["2222"] }] }],
  } as never);
});

async function select(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(
    await screen.findByRole("heading", {
      name,
    }),
  );
}

describe("GroupsPanel", () => {
  it("lists groups with their directories, members and settings", async () => {
    render(<GroupsPanel />);

    expect(await screen.findByRole("heading", { name: "company" })).toBeInTheDocument();
    expect(screen.getByText("ServerAliveInterval 30")).toBeInTheDocument();
    expect(screen.getByText("build01")).toBeInTheDocument();
    expect(screen.getByText("connections/company/ · keys/company/")).toBeInTheDocument();
  });

  it("puts group creation before the group list and shows saving controls only for a draft", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    const addHeading = await screen.findByRole("heading", { name: "Add a group" });
    const list = screen.getByRole("list", { name: "Groups, parent before child" });
    expect(addHeading.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    expect(screen.queryByRole("region", { name: "Unsaved group changes" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Save groups" })).toBeNull();
    expect(screen.getByText(/There are no unsaved changes/)).toBeInTheDocument();

    await user.type(screen.getByLabelText("New group name"), "lab");
    await user.click(screen.getByRole("button", { name: "Add group" }));

    const bar = screen.getByRole("region", { name: "Unsaved group changes" });
    expect(bar).not.toHaveClass("sticky");
    expect(bar).toHaveClass("sm:sticky", "sm:bottom-0");
    expect(within(bar).getByText(/Save or discard them before renaming or removing/)).toBeInTheDocument();
    expect(within(bar).getByRole("button", { name: "Preview group changes" })).toBeEnabled();
    expect(within(bar).getByRole("button", { name: "Discard group changes" })).toBeEnabled();
    expect(within(bar).getByRole("button", { name: "Save groups" })).toBeEnabled();
  });

  it("protects staged edits from immediate group operations and can discard the draft", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    await user.type(await screen.findByLabelText("New group name"), "lab");
    await user.click(screen.getByRole("button", { name: "Add group" }));
    await select(user, "company");

    expect(screen.getByLabelText("Rename company to")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Rename company" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove company" })).toBeDisabled();
    expect(configApi.renameGroup).not.toHaveBeenCalled();
    expect(configApi.deleteGroup).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Discard group changes" }));

    expect(screen.queryByRole("heading", { name: "lab" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Unsaved group changes" })).toBeNull();
    expect(screen.getByText(/There are no unsaved changes/)).toBeInTheDocument();
    expect(screen.getByLabelText("Rename company to")).toBeEnabled();
    expect(screen.getByRole("button", { name: "Remove company" })).toBeEnabled();
  });

  it("adds a nested group by naming its path and saves it", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    expect(await screen.findByRole("heading", { name: "Add a group" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Preview group changes" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Save groups" })).toBeNull();

    await user.type(await screen.findByLabelText("New group name"), "company/work");
    await user.click(screen.getByRole("button", { name: "Add group" }));
    expect(screen.getByRole("button", { name: "Preview group changes" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Save groups" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Preview group changes" }));

    await waitFor(() => expect(configApi.preview).toHaveBeenCalledWith(expect.objectContaining({ kind: "groups" })));
    expect(await screen.findByText(/Port: unset → 2222/)).toBeInTheDocument();

    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1",
      written: ["groups.sshc.conf"],
      preview: { operation: "config.groups", diffs: [] },
    } as never);
    await user.click(screen.getByRole("button", { name: "Save groups" }));

    await waitFor(() =>
      expect(configApi.save).toHaveBeenCalledWith(
        expect.objectContaining({
          kind: "groups",
          metadata: expect.objectContaining({
            groups: expect.arrayContaining([expect.objectContaining({ name: "company/work" })]),
          }),
        }),
      ),
    );
  });

  it("refuses a name that is not a safe relative directory", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    await user.type(await screen.findByLabelText("New group name"), "../escape");
    await user.click(screen.getByRole("button", { name: "Add group" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("relative directory path");
    expect(configApi.save).not.toHaveBeenCalled();
  });

  it("renames a group through the server rather than editing the document", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.renameGroup).mockResolvedValue({
      transactionId: "t1",
      written: ["config"],
      preview: { operation: "config.group_rename", diffs: [] },
    } as never);
    render(<GroupsPanel />);

    await select(user, "company");
    expect(screen.getByText("Rename and remove write to disk immediately.")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Rename company to"), "corp");
    await user.click(screen.getByRole("button", { name: "Rename company" }));

    await waitFor(() => expect(configApi.renameGroup).toHaveBeenCalledWith("company", "corp"));
    expect(configApi.save).not.toHaveBeenCalled();
  });

  it("refuses a rename onto a group that already exists instead of merging", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      metadata: { ...overview.metadata, groups: [{ name: "company" }, { name: "lab" }] },
    } as never);
    render(<GroupsPanel />);

    await select(user, "company");
    await user.type(screen.getByLabelText("Rename company to"), "lab");
    await user.click(screen.getByRole("button", { name: "Rename company" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("lab already exists");
    expect(configApi.renameGroup).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "company" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "lab" })).toBeInTheDocument();
  });

  it("refuses an empty rename", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    await select(user, "company");
    await user.click(screen.getByRole("button", { name: "Rename company" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a new name for the group");
  });

  it("removes a group by naming where its connections go", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      metadata: { ...overview.metadata, groups: [{ name: "company" }, { name: "archive" }] },
    } as never);
    vi.mocked(configApi.deleteGroup).mockResolvedValue({
      transactionId: "t1",
      written: ["config"],
      preview: { operation: "config.group_delete", diffs: [] },
    } as never);
    render(<GroupsPanel />);

    await select(user, "company");

    await user.click(await screen.findByRole("button", { name: "Remove company" }));
    expect(screen.getByText(/takes away its Include line/)).toBeInTheDocument();
    expect(screen.getByText(/No configuration file is deleted/)).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Move its connections to"), "archive");
    await user.click(screen.getByRole("button", { name: "Remove company" }));

    await waitFor(() => expect(configApi.deleteGroup).toHaveBeenCalledWith("company", "archive"));
  });
  it("marks a group that is only in the draft, and will not write files for it", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    await user.type(await screen.findByLabelText(/New group name/i), "lab");
    await user.click(screen.getByRole("button", { name: "Add group" }));

    expect(await screen.findByText("Not saved")).toBeInTheDocument();
    await select(user, "lab");
    expect(screen.getByRole("button", { name: "Remove lab" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Rename lab" })).toBeDisabled();
    expect(screen.getByText(/does not have a directory yet/)).toBeInTheDocument();
    expect(screen.getByText(/not saved yet/)).toBeInTheDocument();
  });

  it("lists a parent before its children", async () => {
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      metadata: {
        ...overview.metadata,
        groups: [{ name: "office/tokyo" }, { name: "hpc" }, { name: "office" }, { name: "office/osaka" }],
      },
    } as never);
    render(<GroupsPanel />);

    await screen.findByRole("heading", { name: "hpc" });
    const list = screen.getByRole("list", { name: "Groups, parent before child" });
    const headings = within(list)
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.textContent);
    expect(headings).toEqual(["hpc", "office", "office/osaka", "office/tokyo"]);
  });

  it("orders siblings by display order and keeps them under their own parent", () => {
    const ordered = treeOrder([
      { name: "office/tokyo" },
      { name: "office", order: 2 },
      { name: "hpc", order: 1 },
      { name: "office/osaka", order: -1 },
    ]).map((group) => group.name);

    expect(ordered).toEqual(["hpc", "office", "office/osaka", "office/tokyo"]);
  });

  it("offers a child group from the group it would sit inside", async () => {
    const user = userEvent.setup();
    render(<GroupsPanel />);

    await select(user, "company");
    await user.click(screen.getByRole("button", { name: "Add a group inside company" }));

    expect(screen.getByLabelText("New group name")).toHaveValue("company/");
    expect(screen.getByLabelText("New group name")).toHaveFocus();
  });
});

describe("hiding a group from the connections tree", () => {


  it("shows a directory that looks like a group but is declared by nothing", async () => {
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      notices: [
        { code: "group_not_declared", detail: "scratch", path: "connections/scratch" },
        { code: "group_empty", detail: "archive", path: "connections/archive" },
      ],
    } as never);
    render(<GroupsPanel />);

    expect(await screen.findByText(/no Include line references it/)).toBeInTheDocument();

    expect(screen.queryByText(/declared and holds nothing/)).toBeNull();
  });
});
