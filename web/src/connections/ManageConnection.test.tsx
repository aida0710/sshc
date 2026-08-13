import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { FileNode, HostDetail } from "../api/config";
import { ManageConnection } from "./ManageConnection";

const detail: HostDetail = {
  form: {
    entry: {
      identity: { path: "connections/work/bastion.conf", alias: "bastion" },
      file: { path: "connections/work/bastion.conf", absolute: "/home/tester/.ssh/connections/work/bastion.conf" },
      line: 1,
      patterns: ["bastion"],
      editable: true,
      group: "work",
    },
    fields: [],
    raw: "Host bastion\n",
    comment: "production edge",
    commentLines: 1,
  },
  metadata: { identity: { path: "connections/work/bastion.conf", alias: "bastion" }, favourite: false },
  effective: { alias: "bastion", entries: [] },
  file: {
    file: { path: "connections/work/bastion.conf", absolute: "/home/tester/.ssh/connections/work/bastion.conf" },
    contents: "# production edge\nHost bastion\n",
    digest: "digest",
    editable: true,
    exists: true,
  },
};

const files = [
  { file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 },
  { file: detail.file.file, editable: true, loads: 1 },
] as FileNode[];

function renderManage(overrides: Partial<Parameters<typeof ManageConnection>[0]> = {}) {
  const props = {
    detail,
    groups: [{ name: "work" }, { name: "personal" }],
    files,
    disabled: false,
    onRename: vi.fn(),
    onMoveToGroup: vi.fn(),
    onComment: vi.fn(),
    onDuplicate: vi.fn(),
    onMoveToFile: vi.fn(),
    onDelete: vi.fn(),
    ...overrides,
  };
  const rendered = render(<ManageConnection {...props} />);
  return { props, ...rendered };
}

describe("ManageConnection", () => {
  it("groups every independent connection-management operation in one region", async () => {
    const user = userEvent.setup();
    const harness = renderManage();

    expect(screen.getByRole("region", { name: "Manage connection" })).toBeInTheDocument();
    await user.clear(screen.getByLabelText("Rename alias"));
    await user.type(screen.getByLabelText("Rename alias"), "gateway");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    expect(harness.props.onRename).toHaveBeenCalledWith("gateway");

    await user.selectOptions(screen.getByLabelText("Primary group"), "personal");
    await user.click(screen.getByRole("button", { name: "Move to this group" }));
    expect(harness.props.onMoveToGroup).toHaveBeenCalledWith("personal");

    await user.clear(screen.getByLabelText("Comment"));
    await user.type(screen.getByLabelText("Comment"), "new comment");
    await user.click(screen.getByRole("button", { name: "Save comment" }));
    expect(harness.props.onComment).toHaveBeenCalledWith("new comment");

    await user.click(screen.getByRole("button", { name: "Duplicate connection" }));
    expect(harness.props.onDuplicate).toHaveBeenCalledOnce();
    await user.selectOptions(screen.getByLabelText("Storage file"), "config");
    await user.click(screen.getByRole("button", { name: "Change storage file" }));
    expect(harness.props.onMoveToFile).toHaveBeenCalledWith("config");

    await user.click(screen.getByRole("button", { name: "Delete connection" }));
    expect(harness.props.onDelete).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Confirm delete" }));
    expect(harness.props.onDelete).toHaveBeenCalledOnce();
  });

  it("disables identity-changing management while another editor is dirty", () => {
    renderManage({ disabled: true });

    const region = screen.getByRole("region", { name: "Manage connection" });
    expect(region).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByLabelText("Rename alias")).toBeDisabled();
    expect(screen.getByLabelText("Primary group")).toBeDisabled();
    expect(screen.getByLabelText("Comment")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Duplicate connection" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete connection" })).toBeDisabled();
    expect(screen.getByText(/Save or discard the editor draft/)).toBeInTheDocument();
  });

  it("resets local management drafts when the committed snapshot changes", async () => {
    const user = userEvent.setup();
    const harness = renderManage();
    await user.clear(screen.getByLabelText("Rename alias"));
    await user.type(screen.getByLabelText("Rename alias"), "staged");
    await user.click(screen.getByRole("button", { name: "Delete connection" }));

    const { group: _group, ...ungroupedEntry } = detail.form.entry;
    const refreshed: HostDetail = {
      ...detail,
      form: {
        ...detail.form,
        entry: { ...ungroupedEntry, identity: { path: "config", alias: "gateway" } },
        comment: "saved comment",
      },
      file: { ...detail.file, contents: "# saved comment\nHost gateway\n", digest: "next" },
    };
    harness.rerender(<ManageConnection {...harness.props} detail={refreshed} />);

    expect(screen.getByLabelText("Rename alias")).toHaveValue("gateway");
    expect(screen.getByLabelText("Primary group")).toHaveValue("");
    expect(screen.getByLabelText("Comment")).toHaveValue("saved comment");
    expect(screen.getByRole("button", { name: "Delete connection" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Confirm delete" })).toBeNull();
  });
});
