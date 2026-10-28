import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { FileNode, HostDetail } from "../api/config";
import { ConnectionActionPanel, type ConnectionAction } from "./ConnectionActionPanel";

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
  metadata: { identity: { path: "connections/work/bastion.conf", alias: "bastion" } },
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

function renderPanel(action: ConnectionAction, overrides: Partial<Parameters<typeof ConnectionActionPanel>[0]> = {}) {
  const props = {
    action,
    detail,
    groups: [{ name: "work" }, { name: "personal" }],
    files,
    disabled: false,
    onRename: vi.fn(async () => true),
    onMoveToGroup: vi.fn(async () => true),
    onComment: vi.fn(async () => true),
    onMoveToFile: vi.fn(async () => true),
    onClose: vi.fn(),
    ...overrides,
  };
  const rendered = render(<ConnectionActionPanel {...props} />);
  return { props, ...rendered };
}

describe("ConnectionActionPanel", () => {
  it("renames the connection and closes once it is saved", async () => {
    const user = userEvent.setup();
    const harness = renderPanel("rename");

    expect(screen.getByRole("button", { name: "Rename" })).toBeDisabled();
    await user.clear(screen.getByLabelText("Rename alias"));
    await user.type(screen.getByLabelText("Rename alias"), "gateway");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    expect(harness.props.onRename).toHaveBeenCalledWith("gateway");
    await waitFor(() => expect(harness.props.onClose).toHaveBeenCalledOnce());
  });

  it("stays open when the save is refused, so the reason and the input are still there", async () => {
    const user = userEvent.setup();
    const harness = renderPanel("group", { onMoveToGroup: vi.fn(async () => false) });

    await user.selectOptions(screen.getByLabelText("Primary group"), "personal");
    await user.click(screen.getByRole("button", { name: "Move to this group" }));
    expect(harness.props.onMoveToGroup).toHaveBeenCalledWith("personal");
    expect(harness.props.onClose).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Primary group")).toHaveValue("personal");
  });

  it("saves the comment and the storage file from their own panels", async () => {
    const user = userEvent.setup();
    const comment = renderPanel("comment");
    await user.clear(screen.getByLabelText("Comment"));
    await user.type(screen.getByLabelText("Comment"), "new comment");
    await user.click(screen.getByRole("button", { name: "Save comment" }));
    expect(comment.props.onComment).toHaveBeenCalledWith("new comment");
    comment.unmount();

    const file = renderPanel("file");
    await user.selectOptions(screen.getByLabelText("Storage file"), "config");
    await user.click(screen.getByRole("button", { name: "Change storage file" }));
    expect(file.props.onMoveToFile).toHaveBeenCalledWith("config");
  });

  it("holds its fields while another editor has a draft", () => {
    renderPanel("rename", { disabled: true });

    expect(screen.getByLabelText("Rename alias")).toBeDisabled();
    expect(screen.getByText(/Save or discard the editor draft/)).toBeInTheDocument();
  });

  it("closes without writing on Cancel", async () => {
    const user = userEvent.setup();
    const harness = renderPanel("rename");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(harness.props.onClose).toHaveBeenCalledOnce();
    expect(harness.props.onRename).not.toHaveBeenCalled();
  });
});
