import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

async function chooseHost(alias: string, scope: HTMLElement = screen.getByRole("tabpanel")) {
  await userEvent.click(within(scope).getByRole("button", { name: "Host" }));
  const label = await screen.findByText(alias, { selector: "span.font-medium" });
  await userEvent.click(label.closest("button")!);
  await userEvent.click(within(scope).getByRole("button", { name: "Connect" }));
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

  it("keeps the add action visible while overflowing tabs scroll", async () => {
    render(<SFTPWorkspace aliases={["edge"]} />);

    const tablist = screen.getByRole("tablist", { name: "Left pane tabs" });
    const add = screen.getByRole("button", { name: "New tab" });
    expect(tablist).toHaveClass("overflow-x-auto");
    expect(tablist).not.toContainElement(add);

    for (let index = 1; index < 8; index += 1) await userEvent.click(add);
    expect(within(tablist).getAllByRole("tab")).toHaveLength(8);
    expect(add).toBeDisabled();
  });

  it("restores tab locations without reconnecting until requested", async () => {
    window.localStorage.setItem("sshc.sftp.tabs", JSON.stringify([
      { alias: "edge", path: "/var/log" },
      { alias: "miyabi", path: "/srv" },
    ]));

    render(<SFTPWorkspace aliases={["edge", "miyabi"]} />);

    expect(api.list).not.toHaveBeenCalled();
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual(["edge:log", "miyabi:srv"]);
    expect(screen.getByText("edge is disconnected")).toBeVisible();

    await userEvent.click(within(screen.getByRole("tabpanel")).getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/var/log"));
    expect(api.list).not.toHaveBeenCalledWith("miyabi", "/srv");
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

    await chooseHost("edge");
    await waitFor(() => expect(currentPath()).toHaveAttribute("data-path", "/home/edge"));
    await userEvent.click(screen.getByRole("button", { name: "Two panes" }));
    const second = screen.getByLabelText("Second remote pane");
    expect(second).toBeInTheDocument();
    expect(window.localStorage.getItem("sshc.sftp.split")).toBe("true");
    expect(within(screen.getByRole("tablist", { name: "Right pane tabs" })).getByRole("tab", { name: "edge:edge" })).toBeVisible();
    expect(within(second).getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "edge");

    await userEvent.click(within(screen.getByRole("tablist", { name: "Right pane tabs" }).parentElement!).getByRole("button", { name: "New tab" }));
    await chooseHost("miyabi", second);
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("miyabi", ""));
    expect(window.localStorage.getItem("sshc.sftp.secondaryTabs")).toContain("miyabi");

    await userEvent.click(screen.getByRole("button", { name: "One pane" }));
    expect(screen.queryByRole("tablist", { name: "Right pane tabs" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Two panes" }));
    const restoredRightTabs = screen.getByRole("tablist", { name: "Right pane tabs" });
    const rightTabs = within(restoredRightTabs).getAllByRole("tab");
    expect(rightTabs).toHaveLength(2);
    expect(rightTabs[1]).toHaveAttribute("aria-selected", "true");
  });

  it("restores independent tabs in both panes", async () => {
    window.localStorage.setItem("sshc.sftp.split", "true");
    window.localStorage.setItem("sshc.sftp.tabs", JSON.stringify([
      { alias: "edge", path: "/var/log" },
    ]));
    window.localStorage.setItem("sshc.sftp.secondaryTabs", JSON.stringify([
      { alias: "miyabi", path: "/srv" },
      { alias: "edge", path: "/tmp" },
    ]));
    window.localStorage.setItem("sshc.sftp.secondaryActiveTab", "1");

    render(<SFTPWorkspace aliases={["edge", "miyabi"]} />);

    const leftTabs = screen.getByRole("tablist", { name: "Left pane tabs" });
    const rightTabs = screen.getByRole("tablist", { name: "Right pane tabs" });
    expect(api.list).not.toHaveBeenCalled();
    await waitFor(() => expect(within(leftTabs).getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["edge:log"]));
    expect(within(rightTabs).getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["miyabi:srv", "edge:tmp"]);
    expect(within(rightTabs).getByRole("tab", { name: "edge:tmp" })).toHaveAttribute("aria-selected", "true");
    expect(within(screen.getByLabelText("Second remote pane")).getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "edge");
    expect(within(screen.getByLabelText("First remote pane")).getByText("edge is disconnected")).toBeVisible();
    expect(within(screen.getByLabelText("Second remote pane")).getByText("edge is disconnected")).toBeVisible();
  });

  it("renders only the primary tab strip and pane on a compact viewport", () => {
    const originalMatchMedia = window.matchMedia;
    window.localStorage.setItem("sshc.sftp.split", "true");
    window.localStorage.setItem("sshc.sftp.secondaryTabs", JSON.stringify([
      { alias: "miyabi", path: "/srv" },
    ]));
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query === "(max-width: 767px)",
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })) as unknown as typeof window.matchMedia;

    try {
      render(<SFTPWorkspace aliases={["edge", "miyabi"]} />);

      expect(screen.getByRole("tablist", { name: "Left pane tabs" })).toBeVisible();
      expect(screen.queryByRole("tablist", { name: "Right pane tabs" })).not.toBeInTheDocument();
      expect(screen.getByLabelText("First remote pane")).toBeVisible();
      expect(screen.queryByLabelText("Second remote pane")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "One pane" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Compare directories" })).not.toBeInTheDocument();
      expect(window.localStorage.getItem("sshc.sftp.split")).toBe("true");
    } finally {
      window.matchMedia = originalMatchMedia;
    }
  });

  it("sends an external target to the visible pane after leaving two-pane mode", async () => {
    const handled = vi.fn();
    const { rerender } = render(
      <SFTPWorkspace aliases={["edge"]} onTargetHandled={handled} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Two panes" }));
    fireEvent.pointerDown(screen.getByLabelText("Second remote pane"));
    await userEvent.click(screen.getByRole("button", { name: "One pane" }));
    rerender(
      <SFTPWorkspace
        aliases={["edge"]}
        target={{ alias: "edge", path: "/var/log", action: "browse", request: 1 }}
        onTargetHandled={handled}
      />,
    );

    await waitFor(() => expect(handled).toHaveBeenCalledWith(1));
    expect(within(screen.getByRole("tabpanel")).getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "edge");
  });
});
