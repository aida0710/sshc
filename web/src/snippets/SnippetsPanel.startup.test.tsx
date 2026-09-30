import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi } from "./api";

vi.mock("./api", () => ({
  snippetsApi: { library: vi.fn(), setStartup: vi.fn() },
}));

const library = {
  snippets: [{ id: "alpha", name: "Alpha", command: "uptime", variables: [] }],
  startup: [],
} as never;

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockReset().mockResolvedValue(library);
  vi.mocked(snippetsApi.setStartup).mockReset().mockResolvedValue(undefined as never);
});

async function chooseAlpha(user: ReturnType<typeof userEvent.setup>) {
  const chooser = await screen.findByRole("combobox", { name: "Snippets" });
  await screen.findByRole("option", { name: "Alpha" });
  await user.selectOptions(chooser, "alpha");
}

it("sets the startup snippet on the first host when the host list arrives after the panel", async () => {
  const user = userEvent.setup();
  const { rerender } = render(<SnippetsPanel aliases={[]} />);
  await chooseAlpha(user);

  rerender(<SnippetsPanel aliases={["web", "db"]} />);
  await user.click(screen.getByRole("button", { name: "Set startup snippet" }));

  expect(snippetsApi.setStartup).toHaveBeenCalledWith("web", "alpha", {});
});

it("falls back to the first host when the chosen host disappears from the list", async () => {
  const user = userEvent.setup();
  const { rerender } = render(<SnippetsPanel aliases={["web", "db"]} />);
  await chooseAlpha(user);
  await user.selectOptions(screen.getByRole("combobox", { name: "Host for the startup snippet" }), "db");

  rerender(<SnippetsPanel aliases={["web"]} />);
  await user.click(screen.getByRole("button", { name: "Set startup snippet" }));

  expect(snippetsApi.setStartup).toHaveBeenCalledWith("web", "alpha", {});
});

const snippets = [
  { id: "one", name: "Check disk", command: "df -h", variables: [] },
];

it("shows a startup assignment as stopped once its destination changed", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets,
    startup: [{ alias: "test-host", snippetId: "one", stale: true }],
  } as never);
  render(<SnippetsPanel aliases={["test-host"]} />);
  expect(
    await screen.findByText(
      /The destination, the authentication settings, or the VPN profile changed after this snippet was assigned/,
    ),
  ).toBeInTheDocument();
});

it("does not warn about a startup assignment whose destination is unchanged", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets,
    startup: [{ alias: "test-host", snippetId: "one", stale: false }],
  } as never);
  render(<SnippetsPanel aliases={["test-host"]} />);
  await screen.findByRole("option", { name: "Check disk" });
  expect(screen.queryByText(/changed after this snippet was assigned/)).not.toBeInTheDocument();
  expect(screen.queryByRole("list", { name: "Hosts with a stopped assignment" }))
    .not.toBeInTheDocument();
});

// 停止中の割り当ては、ドロップダウンでそのホストを選ばなくても、どのホストのものか分かる。
// 更新で古い形式の割り当てがまとめて停止中になっても、割り当て直す先を探せる。
it("lists every host whose startup assignment stopped, whichever host is selected", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets,
    startup: [
      { alias: "web", snippetId: "one", stale: true },
      { alias: "db", snippetId: "one", stale: false },
      { alias: "batch", snippetId: "one", stale: true },
    ],
  } as never);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["db", "web", "batch"]} />);

  const stopped = await screen.findByRole("list", { name: "Hosts with a stopped assignment" });
  expect(within(stopped).getAllByRole("listitem").map((item) => item.textContent))
    .toEqual(["web", "batch"]);
  expect(screen.getByText(/Stopped assignments: 2\./))
    .toBeInTheDocument();
  expect(screen.queryByText(/changed after this snippet was assigned/)).not.toBeInTheDocument();

  const hosts = screen.getAllByRole("combobox").find((select) =>
    within(select).queryByRole("option", { name: "batch" }) !== null,
  );
  if (hosts === undefined) throw new Error("the startup host selector is missing");
  await user.selectOptions(hosts, "batch");
  expect(screen.getByText(/changed after this snippet was assigned/)).toBeInTheDocument();
});
