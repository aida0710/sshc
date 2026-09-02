import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SFTPWorkspace } from "./SFTPWorkspace";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  readText: vi.fn(),
  previewFile: vi.fn(),
  listTransfers: vi.fn(),
  clearFinishedTransfers: vi.fn(),
}));

vi.mock("./api", () => ({ sftpApi: api }));
vi.mock("../api/integrations", () => ({
  integrationsApi: { recentConnections: vi.fn(async () => ({ connections: [] })) },
}));

async function chooseHost(alias: string) {
  await userEvent.click(within(screen.getByRole("tabpanel")).getByRole("button", { name: "Host" }));
  const label = await screen.findByText(alias, { selector: "span.font-medium" });
  await userEvent.click(label.closest("button")!);
}

function currentPath(): HTMLElement {
  return within(screen.getByRole("tabpanel")).getByTestId("sftp-current-path");
}

describe("SFTP tabs", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.clearAllMocks();
    api.list.mockImplementation(async (_alias: string, requestedPath: string) => ({
      path: requestedPath === "" ? "/home/edge" : requestedPath,
      entries: [],
    }));
    api.listTransfers.mockResolvedValue({
      maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: false,
      largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, jobs: [],
    });
  });

  it("keeps each tab on its own host and directory", async () => {
    render(<SFTPWorkspace aliases={["edge", "miyabi"]} />);

    await chooseHost("edge");
    await waitFor(() => expect(currentPath()).toHaveAttribute("data-path", "/home/edge"));

    await userEvent.click(screen.getByRole("button", { name: "New tab" }));
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(2);
    expect(tabs[1]).toHaveAttribute("aria-selected", "true");
    // A second panel starts unconnected rather than inheriting the first host.
    expect(within(screen.getByRole("tabpanel")).getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "");

    await chooseHost("miyabi");
    await userEvent.click(within(screen.getByRole("tabpanel")).getByRole("button", { name: "Edit path" }));
    const remotePath = within(screen.getByRole("tabpanel")).getByRole("textbox", { name: "Remote path" });
    await userEvent.clear(remotePath);
    await userEvent.type(remotePath, "/srv{Enter}");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("miyabi", "/srv"));

    await userEvent.click(screen.getAllByRole("tab")[0]!);
    expect(currentPath()).toHaveAttribute("data-path", "/home/edge");
    expect(within(screen.getByRole("tabpanel")).getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "edge");
  });

  it("names a tab after its host and directory and closes it again", async () => {
    render(<SFTPWorkspace aliases={["edge"]} />);
    expect(screen.getByRole("tab", { name: "New tab" })).toBeVisible();
    expect(screen.queryByRole("button", { name: /Close the/ })).not.toBeInTheDocument();

    await chooseHost("edge");
    await waitFor(() => expect(screen.getByRole("tab", { name: "edge:edge" })).toBeVisible());

    await userEvent.click(screen.getByRole("button", { name: "New tab" }));
    expect(screen.getAllByRole("tab")).toHaveLength(2);

    await userEvent.click(screen.getByRole("button", { name: "Close the New tab tab" }));
    const remaining = screen.getAllByRole("tab");
    expect(remaining).toHaveLength(1);
    expect(remaining[0]).toHaveAccessibleName("edge:edge");
  });

  it("reopens the directories that were open when the app last closed", async () => {
    window.localStorage.setItem("sshc.sftp.tabs", JSON.stringify([
      { alias: "edge", path: "/var/log" },
      { alias: "miyabi", path: "/srv" },
    ]));

    render(<SFTPWorkspace aliases={["edge", "miyabi"]} />);

    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/var/log"));
    expect(api.list).toHaveBeenCalledWith("miyabi", "/srv");
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual(["edge:log", "miyabi:srv"]);
  });

  it("ignores remembered tabs whose host is no longer declared", async () => {
    window.localStorage.setItem("sshc.sftp.tabs", JSON.stringify([{ alias: "removed", path: "/gone" }]));

    render(<SFTPWorkspace aliases={["edge"]} />);

    await waitFor(() => expect(screen.getByRole("tab", { name: "removed:gone" })).toBeVisible());
    expect(api.list).not.toHaveBeenCalled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("opens a persistent second remote pane on desktop", async () => {
    render(<SFTPWorkspace aliases={["edge", "miyabi"]} />);

    await userEvent.click(screen.getByRole("button", { name: "Two panes" }));
    const second = screen.getByLabelText("Second remote pane");
    expect(second).toBeInTheDocument();
    expect(window.localStorage.getItem("sshc.sftp.split")).toBe("true");

    await userEvent.click(within(second).getByRole("button", { name: "Host" }));
    const option = await screen.findByText("miyabi", { selector: "span.font-medium" });
    await userEvent.click(option.closest("button")!);
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("miyabi", ""));
    expect(window.localStorage.getItem("sshc.sftp.secondary")).toContain("miyabi");
  });
});
