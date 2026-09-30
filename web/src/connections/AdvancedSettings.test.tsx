import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostDetail } from "../api/config";
import { AdvancedSettings } from "./AdvancedSettings";

const detail: HostDetail = {
  form: {
    entry: {
      identity: { path: "config", alias: "bastion" },
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      line: 1,
      patterns: ["bastion"],
      editable: true,
    },
    fields: [
      { line: 2, keyword: "ProxyJump", values: ["edge"], category: "jump", editable: true },
      { line: 3, keyword: "Compression", values: ["yes"], category: "advanced", editable: true },
    ],
    raw: "Host bastion\n\tProxyJump edge\n\tCompression yes\n",
    comment: "",
    commentLines: 0,
  },
  metadata: { identity: { path: "config", alias: "bastion" } },
  effective: { alias: "bastion", entries: [] },
  file: {
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    contents: "Host bastion\n\tProxyJump edge\n\tCompression yes\n",
    digest: "digest",
    editable: true,
    exists: true,
  },
};

function renderAdvanced(area: "Jump" | "Forwards" | "Directives" | "Raw" = "Raw", selected = detail) {
  const props = {
    detail: selected,
    area,
    onAreaChange: vi.fn(),
    onFieldEdits: vi.fn(),
    onBlockRaw: vi.fn(),
    disabled: false,
    onDirtyChange: vi.fn(),
  };
  const rendered = render(<AdvancedSettings {...props} />);
  return { props, ...rendered };
}

describe("AdvancedSettings", () => {
  it("keeps a Raw draft across internal views and blocks another editor until discard", async () => {
    const user = userEvent.setup();
    const harness = renderAdvanced();
    const raw = screen.getByLabelText(/Block text/);
    await user.clear(raw);
    await user.type(raw, "Host bastion\n\tPort 2200\n");
    expect(harness.props.onDirtyChange).toHaveBeenCalledWith(true);

    harness.rerender(<AdvancedSettings {...harness.props} area="Jump" />);
    expect(screen.getByLabelText("ProxyJump")).toBeDisabled();
    expect(screen.getByText(/Raw has unsaved changes/)).toBeInTheDocument();

    harness.rerender(<AdvancedSettings {...harness.props} area="Raw" />);
    expect(screen.getByLabelText(/Block text/)).toHaveValue("Host bastion\n\tPort 2200\n");
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    expect(screen.getByLabelText(/Block text/)).toHaveValue(detail.form.raw);
  });

  it("keeps Raw read-only while a directive draft owns the editor", async () => {
    const user = userEvent.setup();
    const harness = renderAdvanced("Jump");
    await user.clear(screen.getByLabelText("ProxyJump"));
    await user.type(screen.getByLabelText("ProxyJump"), "gateway");

    harness.rerender(<AdvancedSettings {...harness.props} area="Raw" />);
    expect(screen.getByLabelText(/Block text/)).toBeDisabled();
    expect(screen.getByText(/Directives have unsaved changes/)).toBeInTheDocument();
  });

  it("sends semantic field edits only from the active directive draft", async () => {
    const user = userEvent.setup();
    const harness = renderAdvanced("Jump");
    await user.clear(screen.getByLabelText("ProxyJump"));
    await user.type(screen.getByLabelText("ProxyJump"), "gateway");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(harness.props.onFieldEdits).toHaveBeenCalledWith([
      { action: "set", line: 2, values: ["gateway"] },
    ]);
    expect(harness.props.onBlockRaw).not.toHaveBeenCalled();
  });

  it("counts a Raw draft as saved as soon as the save succeeds, before the host is loaded again", async () => {
    const user = userEvent.setup();
    const harness = renderAdvanced();
    harness.props.onBlockRaw.mockResolvedValue(undefined);
    await user.type(screen.getByLabelText(/Block text/), "\tPort 2200\n");
    await user.click(screen.getByRole("button", { name: "Save block" }));

    expect(harness.props.onBlockRaw).toHaveBeenCalledWith(`${detail.form.raw}\tPort 2200\n`);
    expect(screen.queryByRole("button", { name: "Save block" })).toBeNull();
    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(false);
    expect(screen.getByLabelText(/Block text/)).toHaveValue(`${detail.form.raw}\tPort 2200\n`);
  });

  it("keeps a directive draft to save again when the save fails", async () => {
    const user = userEvent.setup();
    const harness = renderAdvanced("Jump");
    harness.props.onFieldEdits.mockRejectedValue(new Error("stale_base"));
    await user.clear(screen.getByLabelText("ProxyJump"));
    await user.type(screen.getByLabelText("ProxyJump"), "gateway");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(screen.getByLabelText("ProxyJump")).toHaveValue("gateway");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeEnabled();
    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);
  });

  it("says which directives a previous line in the block already decided", () => {
    const duplicated: HostDetail = {
      ...detail,
      form: {
        ...detail.form,
        fields: [
          ...detail.form.fields,
          { line: 4, keyword: "Compression", values: ["no"], category: "advanced", editable: true, duplicate: true },
        ],
      },
    };
    render(
      <AdvancedSettings
        detail={duplicated}
        area="Directives"
        onAreaChange={vi.fn()}
        onFieldEdits={vi.fn()}
        onBlockRaw={vi.fn()}
        disabled={false}
        onDirtyChange={vi.fn()}
      />,
    );

    expect(screen.getByText(/OpenSSH keeps the first one/)).toBeInTheDocument();
  });

  it("keeps the single quotes of a command the user edits, because the shell reads it", async () => {
    const user = userEvent.setup();
    const withCommand: HostDetail = {
      ...detail,
      form: {
        ...detail.form,
        fields: [
          ...detail.form.fields,
          { line: 4, keyword: "RemoteCommand", values: ["awk '{print $1}' /etc/passwd"], category: "advanced", editable: true },
        ],
      },
    };
    const harness = renderAdvanced("Directives", withCommand);
    const command = screen.getByLabelText("RemoteCommand");
    expect(command).toHaveValue("awk '{print $1}' /etc/passwd");

    await user.clear(command);
    await user.click(command);
    await user.paste("awk -F: '{print $1}' /etc/passwd");
    await user.click(screen.getByLabelText("New directive"));
    await user.paste("LocalCommand");
    await user.click(screen.getByLabelText("New value"));
    await user.paste("sh -c 'echo $PPID'");
    await user.click(screen.getByRole("button", { name: "Add directive" }));
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(harness.props.onFieldEdits).toHaveBeenCalledWith([
      { action: "set", line: 4, values: ["awk -F: '{print $1}' /etc/passwd"] },
      { action: "add", keyword: "LocalCommand", values: ["sh -c 'echo $PPID'"] },
    ]);
  });

  it("adds and removes Local and Dynamic forwarding through semantic edits", async () => {
    const forwarded: HostDetail = {
      ...detail,
      form: {
        ...detail.form,
        fields: [
          ...detail.form.fields,
          { line: 4, keyword: "DynamicForward", values: ["1080"], category: "advanced", editable: true },
        ],
      },
    };
    const user = userEvent.setup();
    const harness = renderAdvanced("Forwards", forwarded);
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.type(screen.getByLabelText("Local port"), "8080");
    await user.type(screen.getByLabelText("Destination"), "db.internal:5432");
    await user.click(screen.getByRole("button", { name: "Add forwarding" }));
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(harness.props.onFieldEdits).toHaveBeenCalledWith([
      { action: "remove", line: 4 },
      { action: "add", keyword: "LocalForward", values: ["8080", "db.internal:5432"] },
    ]);
  });

  it("hides the fixed destination for a Dynamic SOCKS proxy", async () => {
    const user = userEvent.setup();
    renderAdvanced("Forwards");

    const destination = screen.getByLabelText("Destination");
    expect(destination).toHaveAttribute("placeholder", "127.0.0.1:5432");
    expect(destination.closest("label")).not.toHaveTextContent("Enter the host name or IP address and port as seen from the SSH server.");
    expect(screen.getByText("Enter the host name or IP address and port as seen from the SSH server.")).toBeVisible();
    await user.selectOptions(screen.getByLabelText("Type"), "dynamic");
    expect(screen.queryByLabelText("Destination")).not.toBeInTheDocument();
    expect(screen.getByText("The application using this SOCKS proxy chooses the destination for each connection.")).toBeVisible();
  });
});
